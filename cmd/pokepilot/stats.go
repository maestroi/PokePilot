package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
)

// runStats is the statistics blob the watch page renders above the trace.
// It answers the question the trace cannot: not "what happened next" but
// "what is this model DOING with its rounds" — how often it re-picks an
// objective it has already picked (Repeats), what it keeps picking
// (Choices), how long it is thinking, what it is spending, and how many
// replies never resolved at all.
//
// Repeats is the headline number. A run that wanders looks fine line by
// line — every round is a legal objective that succeeds — and only the
// tally shows it walked between the same four places for eighteen rounds.
// The tally's wire type lives in farm: it rides the heartbeat to the wall,
// so the console and the watch page render one definition. The aliases keep
// this file and its tests on the old names.
type (
	runStats    = farm.LLMStats
	choiceCount = farm.ChoiceCount
)

const strategicReplanAfter = 4

// statsPlanner wraps a planner and tallies what it chooses, pushing the
// tally to the watch page after every ask. It is a decorator rather than
// bookkeeping inside agent.Run because the numbers it wants are all in the
// call it already wraps — the observation going in, the objective coming
// out, the wall clock around it — so agent stays exactly as it was.
//
// This is also the run-level structured-goal seam. Both local and farm LLM
// runs already pass through statsPlanner after assigning LLMPlanner.Goal.
// When that existing Goal uses agent.ParseGoal's structured syntax, evaluate
// it before spending a model call and return ErrDone once RAM/state proves
// completion. Free-text Goal values are untouched and remain prompt-only.
// While a structured goal is incomplete, its deterministic GoalStatus is
// also rendered as a progress-only system note and copied onto LLMStats so
// the planner and operator see the same state. The note reports facts; it
// never prescribes a route or strategy.
//
// The same decorator owns a derived long-horizon progress tracker. It uses
// StrategicMemory only for observable progress/no-progress accounting; the
// planner's existing Observation.Intent remains the sole planner-owned
// strategy sentence. Once progress has stalled for several distinct rounds,
// a temporary system note asks for a materially different approach. The
// signal never chooses that approach and is not persisted as a second memory
// slot. Retries of the same round do not advance the counter.
//
// Endpoint selection is deliberately not another stats concern. The optional
// POKEPILOT_LLM_FALLBACK_* endpoint is configured here, then agent's
// FailoverPlanner owns transport-only failover and permanent pinning. Its
// per-endpoint call hook is the only routing seam statsPlanner needs.
type statsPlanner struct {
	inner  *agent.LLMPlanner
	router *agent.FailoverPlanner

	emu  *emu.Emu       // for stall RAM captures; nil in unit tests
	push func(any)      // emu.TraceStats
	snap *heartbeatSnap // farm heartbeat; nil on the local (non-farm) run

	stats   runStats
	counts  map[string]int
	offered int           // summed over calls, for the average
	elapsed time.Duration // summed over calls, for the average

	strategy        agent.StrategicMemory
	strategyRound   int
	strategySeen    bool
	stallCaptured   bool
	baseExtraSystem string
}

func newStatsPlanner(inner *agent.LLMPlanner, m *emu.Emu, push func(any), snap *heartbeatSnap) *statsPlanner {
	fallbackDefaults := agent.LLMConfig{
		Model:     inner.Model,
		Token:     inner.Token,
		NoThink:   inner.NoThink,
		MaxTokens: inner.MaxTokens,
	}
	fallbackConfig, configured := agent.OptionalLLMConfigFromEnv("POKEPILOT_LLM_FALLBACK_", fallbackDefaults)
	var fallback *agent.LLMPlanner
	if configured {
		fallback = agent.NewLLMPlannerFromConfig(fallbackConfig)
	}

	s := &statsPlanner{
		inner:           inner,
		emu:             m,
		push:            push,
		snap:            snap,
		counts:          map[string]int{},
		baseExtraSystem: inner.ExtraSystem,
	}
	s.router = agent.NewFailoverPlanner(inner, fallback)
	s.router.OnCall = func(call agent.LLMCall) {
		s.record(call.Observation, call.Offered, call.Objective, call.Err, call.Duration)
	}
	return s
}

func (s *statsPlanner) Next(obs agent.Observation, offered []agent.Objective) (agent.Objective, error) {
	done, err := s.prepareRunContext(obs)
	if err != nil {
		return agent.Objective{}, err
	}
	if done {
		s.publishSnapshot(obs)
		return agent.Objective{}, agent.ErrDone
	}
	return s.ask(obs, offered, nil)
}

func (s *statsPlanner) NextRetry(obs agent.Observation, offered []agent.Objective, r agent.Retry) (agent.Objective, error) {
	done, err := s.prepareRunContext(obs)
	if err != nil {
		return agent.Objective{}, err
	}
	if done {
		s.publishSnapshot(obs)
		return agent.Objective{}, agent.ErrDone
	}
	return s.ask(obs, offered, &r)
}

func (s *statsPlanner) ask(obs agent.Observation, offered []agent.Objective, retry *agent.Retry) (agent.Objective, error) {
	if retry == nil {
		return s.router.Next(obs, offered)
	}
	return s.router.NextRetry(obs, offered, *retry)
}

// prepareRunContext is the one per-ask seam for run-derived context. Goal
// evaluation happens first so malformed structured syntax fails before any
// other state is mutated. The same observation then feeds the stall tracker.
func (s *statsPlanner) prepareRunContext(obs agent.Observation) (bool, error) {
	status, structured, err := agent.PlannerGoalStatus(s.inner.Goal, obs)
	if err != nil {
		return false, err
	}
	s.setGoalStats(status, structured)
	s.prepareStrategyWithGoal(obs, status, structured)
	return structured && status.Complete, nil
}

func (s *statsPlanner) setGoalStats(status agent.GoalStatus, structured bool) {
	if !structured {
		s.stats.GoalSummary = ""
		s.stats.GoalCurrent = 0
		s.stats.GoalTarget = 0
		s.stats.GoalComplete = false
		return
	}
	s.stats.GoalSummary = status.Summary
	s.stats.GoalCurrent = status.Current
	s.stats.GoalTarget = status.Target
	s.stats.GoalComplete = status.Complete
}

func (s *statsPlanner) prepareStrategyWithGoal(obs agent.Observation, goal agent.GoalStatus, structuredGoal bool) {
	// NextRetry receives the same observation and round as Next. Count a
	// world state once, not once per model attempt.
	if !s.strategySeen || obs.Round != s.strategyRound {
		s.strategy.ObserveProgress(obs)
		s.strategyRound = obs.Round
		s.strategySeen = true
	}

	extra := s.baseExtraSystem
	if structuredGoal && goal.Summary != "" {
		extra = appendSystemNote(extra, "RUN GOAL STATUS: "+goal.Summary+". This is observable progress only, not a prescribed strategy.")
	}
	reason := s.strategy.ReplanReason(strategicReplanAfter, obs.Intent)
	if reason != "" {
		extra = appendSystemNote(extra, "RUN REPLAN SIGNAL: "+reason)
	}
	// Preserve RAM on the EDGE of the stall, not every stalled round. This is
	// the pathology objective-failure forensics cannot see: nothing failed,
	// every objective returned done, and the run still went nowhere.
	switch {
	case reason == "":
		s.stallCaptured = false
	case !s.stallCaptured:
		s.stallCaptured = true
		if err := agent.CaptureStall(s.emu, obs.Intent, reason); err != nil {
			fmt.Printf("  ram forensics: %v\n", err)
		}
	}
	s.inner.ExtraSystem = extra
}

func appendSystemNote(base, note string) string {
	if base == "" {
		return note
	}
	return base + "\n\n" + note
}

// record folds one endpoint ask into the tally and publishes it. A re-ask
// after a rejection counts as a call and as a rejection, never as a round:
// the round is the same one, asked again. On failover the router calls this
// once for the failed primary and once for the fallback.
func (s *statsPlanner) record(obs agent.Observation, offered int, o agent.Objective, err error, took time.Duration) {
	s.stats.Calls++
	s.offered += offered
	s.elapsed += took
	s.stats.LastSeconds = took.Seconds()
	s.stats.AvgOffered = float64(s.offered) / float64(s.stats.Calls)
	s.stats.AvgSeconds = s.elapsed.Seconds() / float64(s.stats.Calls)
	s.stats.Round, s.stats.RoundsLeft = obs.Round, obs.RoundsLeft
	s.stats.Intent, s.stats.IntentAge = obs.Intent, obs.IntentAge
	route := s.router.Route()
	s.stats.Backend, s.stats.Model, s.stats.Failovers = route.Backend, route.Model, route.Failovers

	h := s.router.Health()
	s.stats.PromptTokens, s.stats.CompletionTokens = h.PromptTokens, h.CompletionTokens
	s.stats.Transport, s.stats.Fallbacks = h.Transport, h.Fallbacks

	if err != nil {
		s.stats.Rejected++
	} else {
		s.stats.Rounds++
		name := o.String()
		if s.counts[name] > 0 {
			s.stats.Repeats++
		}
		s.counts[name]++
		s.stats.Choices = rankChoices(s.counts)
	}
	s.publish()
}

// publishSnapshot pushes run-derived state without inventing a model call.
// Structured goal completion uses it because the stop happens before the
// next LLM ask; operators should still see the final Complete=true status.
func (s *statsPlanner) publishSnapshot(obs agent.Observation) {
	s.stats.Round, s.stats.RoundsLeft = obs.Round, obs.RoundsLeft
	s.stats.Intent, s.stats.IntentAge = obs.Intent, obs.IntentAge
	route := s.router.Route()
	s.stats.Backend, s.stats.Model, s.stats.Failovers = route.Backend, route.Model, route.Failovers
	h := s.router.Health()
	s.stats.PromptTokens, s.stats.CompletionTokens = h.PromptTokens, h.CompletionTokens
	s.stats.Transport, s.stats.Fallbacks = h.Transport, h.Fallbacks
	s.publish()
}

func (s *statsPlanner) publish() {
	if s.snap != nil {
		s.snap.storeStats(s.stats)
	}
	if s.push != nil {
		s.push(s.stats)
	}
}

// Usage makes the live local path preserve agent.Run's model-spend totals
// even though statsPlanner sits between Run and the concrete LLM planners.
func (s *statsPlanner) Usage() (prompt, completion int) {
	return s.router.Usage()
}

// rankChoices orders the tally most-chosen first, ties by name so the panel
// does not reshuffle itself between polls.
func rankChoices(counts map[string]int) []choiceCount {
	out := make([]choiceCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, choiceCount{Objective: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Objective < out[j].Objective
	})
	return out
}
