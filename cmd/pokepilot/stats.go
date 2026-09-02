package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/agent"
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
// Finally, an optional fallback endpoint can be configured with
// POKEPILOT_LLM_FALLBACK_*. Failover happens ONLY when the active planner's
// LLMHealth.Transport counter rises during the ask: a bad model choice or
// malformed reply stays with the same model and follows the ordinary reply
// retry path. After one primary transport failure, the fallback is pinned for
// the rest of the run so later rounds and reply retries do not bounce between
// models.
type statsPlanner struct {
	inner     *agent.LLMPlanner
	fallback  *agent.LLMPlanner
	active    *agent.LLMPlanner
	backend   string
	failovers int

	push func(any)      // emu.TraceStats
	snap *heartbeatSnap // farm heartbeat; nil on the local (non-farm) run

	stats   runStats
	counts  map[string]int
	offered int           // summed over calls, for the average
	elapsed time.Duration // summed over calls, for the average

	strategy        agent.StrategicMemory
	strategyRound   int
	strategySeen    bool
	baseExtraSystem string
}

func newStatsPlanner(inner *agent.LLMPlanner, push func(any), snap *heartbeatSnap) *statsPlanner {
	s := &statsPlanner{
		inner:           inner,
		active:          inner,
		backend:         "primary",
		push:            push,
		snap:            snap,
		counts:          map[string]int{},
		baseExtraSystem: inner.ExtraSystem,
	}
	s.fallback = fallbackPlannerFromEnv(inner)
	return s
}

func fallbackPlannerFromEnv(primary *agent.LLMPlanner) *agent.LLMPlanner {
	baseURL := strings.TrimSpace(os.Getenv("POKEPILOT_LLM_FALLBACK_URL"))
	if baseURL == "" {
		return nil
	}
	model := strings.TrimSpace(os.Getenv("POKEPILOT_LLM_FALLBACK_MODEL"))
	if model == "" {
		model = primary.Model
	}
	p := &agent.LLMPlanner{
		BaseURL:     baseURL,
		Model:       model,
		Goal:        primary.Goal,
		ExtraSystem: primary.ExtraSystem,
		NoThink:     envBoolOr("POKEPILOT_LLM_FALLBACK_NO_THINK", primary.NoThink),
		MaxTokens:   primary.MaxTokens,
		Log:         primary.Log,
		PromptLog:   primary.PromptLog,
		ReplyLog:    primary.ReplyLog,
	}
	if token, ok := os.LookupEnv("POKEPILOT_LLM_FALLBACK_TOKEN"); ok {
		p.Token = token
	} else {
		p.Token = primary.Token
	}
	if v := strings.TrimSpace(os.Getenv("POKEPILOT_LLM_FALLBACK_MAX_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.MaxTokens = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("POKEPILOT_LLM_FALLBACK_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			p.Timeout = d
		}
	}
	return p
}

func envBoolOr(name string, fallback bool) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	return v != "" && v != "0"
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
	p := s.active
	backend := s.backend
	s.syncPlannerContext(p)
	o, err, transport := s.callPlanner(p, backend, obs, offered, retry)
	if transport && p == s.inner && s.fallback != nil {
		s.failovers++
		s.active = s.fallback
		s.backend = "fallback"
		if s.inner.Log != nil {
			fmt.Fprintf(s.inner.Log,
				"  llm route: primary %s at %s had a transport failure; pinning fallback %s at %s for the rest of the run\n",
				s.inner.Model, s.inner.BaseURL, s.fallback.Model, s.fallback.BaseURL)
		}
		p = s.fallback
		backend = s.backend
		s.syncPlannerContext(p)
		o, err, transport = s.callPlanner(p, backend, obs, offered, retry)
	}
	if err != nil && transport {
		return agent.Objective{}, fmt.Errorf("%w: %v", agent.ErrTransport, err)
	}
	return o, err
}

func (s *statsPlanner) callPlanner(p *agent.LLMPlanner, backend string, obs agent.Observation, offered []agent.Objective, retry *agent.Retry) (agent.Objective, error, bool) {
	beforeTransport := p.Health.Transport
	start := time.Now()
	var (
		o   agent.Objective
		err error
	)
	if retry == nil {
		o, err = p.Next(obs, offered)
	} else {
		o, err = p.NextRetry(obs, offered, *retry)
	}
	s.recordFrom(p, backend, obs, len(offered), o, err, time.Since(start))
	return o, err, p.Health.Transport > beforeTransport
}

func (s *statsPlanner) syncPlannerContext(p *agent.LLMPlanner) {
	if p == nil || p == s.inner {
		return
	}
	p.Goal = s.inner.Goal
	p.ExtraSystem = s.inner.ExtraSystem
	p.Log = s.inner.Log
	p.PromptLog = s.inner.PromptLog
	p.ReplyLog = s.inner.ReplyLog
}

func (s *statsPlanner) goalDone(obs agent.Observation) (bool, error) {
	status, structured, err := agent.PlannerGoalStatus(s.inner.Goal, obs)
	if err != nil {
		return false, err
	}
	return structured && status.Complete, nil
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

func (s *statsPlanner) prepareStrategy(obs agent.Observation) {
	s.prepareStrategyWithGoal(obs, agent.GoalStatus{}, false)
}

func (s *statsPlanner) prepareStrategyWithGoal(obs agent.Observation, goal agent.GoalStatus, structuredGoal bool) {
	// NextRetry receives the same observation and round as Next. Count a
	// world state once, not once per model attempt.
	if !s.strategySeen || obs.Round != s.strategyRound {
		s.strategy.ObserveProgress(obs, 0)
		s.strategyRound = obs.Round
		s.strategySeen = true
	}

	extra := s.baseExtraSystem
	if structuredGoal && goal.Summary != "" {
		extra = appendSystemNote(extra, "RUN GOAL STATUS: "+goal.Summary+". This is observable progress only, not a prescribed strategy.")
	}
	if reason := s.strategy.ReplanReason(strategicReplanAfter, obs.Intent); reason != "" {
		extra = appendSystemNote(extra, "RUN REPLAN SIGNAL: "+reason)
	}
	s.inner.ExtraSystem = extra
}

func appendSystemNote(base, note string) string {
	if base == "" {
		return note
	}
	return base + "\n\n" + note
}

// record folds one ask into the tally and publishes it. A re-ask after a
// rejection counts as a call and as a rejection, never as a round: the
// round is the same one, asked again.
func (s *statsPlanner) record(obs agent.Observation, offered int, o agent.Objective, err error, took time.Duration) {
	s.recordFrom(s.active, s.backend, obs, offered, o, err, took)
}

func (s *statsPlanner) recordFrom(p *agent.LLMPlanner, backend string, obs agent.Observation, offered int, o agent.Objective, err error, took time.Duration) {
	s.stats.Calls++
	s.offered += offered
	s.elapsed += took
	s.stats.LastSeconds = took.Seconds()
	s.stats.AvgOffered = float64(s.offered) / float64(s.stats.Calls)
	s.stats.AvgSeconds = s.elapsed.Seconds() / float64(s.stats.Calls)
	s.stats.Round, s.stats.RoundsLeft = obs.Round, obs.RoundsLeft
	s.stats.Intent, s.stats.IntentAge = obs.Intent, obs.IntentAge
	s.stats.Backend, s.stats.Model, s.stats.Failovers = backend, p.Model, s.failovers

	h := s.health()
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
	s.stats.Failovers = s.failovers
	if s.active != nil {
		s.stats.Backend, s.stats.Model = s.backend, s.active.Model
	}
	h := s.health()
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

func (s *statsPlanner) health() agent.LLMHealth {
	h := s.inner.Health
	if s.fallback == nil {
		return h
	}
	f := s.fallback.Health
	h.Transport += f.Transport
	h.Rejected += f.Rejected
	h.Fallbacks += f.Fallbacks
	h.PromptTokens += f.PromptTokens
	h.CompletionTokens += f.CompletionTokens
	return h
}

// Usage makes the live local path preserve agent.Run's model-spend totals
// even though statsPlanner sits between Run and the concrete LLMPlanner.
func (s *statsPlanner) Usage() (prompt, completion int) {
	h := s.health()
	return h.PromptTokens, h.CompletionTokens
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
