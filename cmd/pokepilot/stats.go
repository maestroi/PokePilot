package main

import (
	"sort"
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
//
// The same decorator owns a derived long-horizon progress tracker. It uses
// StrategicMemory only for observable progress/no-progress accounting; the
// planner's existing Observation.Intent remains the sole planner-owned
// strategy sentence. Once progress has stalled for several distinct rounds,
// a temporary system note asks for a materially different approach. The
// signal never chooses that approach and is not persisted as a second memory
// slot. Retries of the same round do not advance the counter.
type statsPlanner struct {
	inner *agent.LLMPlanner
	push  func(any)      // emu.TraceStats
	snap  *heartbeatSnap // farm heartbeat; nil on the local (non-farm) run

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
	return &statsPlanner{
		inner:           inner,
		push:            push,
		snap:            snap,
		counts:          map[string]int{},
		baseExtraSystem: inner.ExtraSystem,
	}
}

func (s *statsPlanner) Next(obs agent.Observation, offered []agent.Objective) (agent.Objective, error) {
	if done, err := s.goalDone(obs); err != nil || done {
		if err != nil {
			return agent.Objective{}, err
		}
		return agent.Objective{}, agent.ErrDone
	}
	s.prepareStrategy(obs)
	start := time.Now()
	o, err := s.inner.Next(obs, offered)
	s.record(obs, len(offered), o, err, time.Since(start))
	return o, err
}

func (s *statsPlanner) NextRetry(obs agent.Observation, offered []agent.Objective, r agent.Retry) (agent.Objective, error) {
	if done, err := s.goalDone(obs); err != nil || done {
		if err != nil {
			return agent.Objective{}, err
		}
		return agent.Objective{}, agent.ErrDone
	}
	s.prepareStrategy(obs)
	start := time.Now()
	o, err := s.inner.NextRetry(obs, offered, r)
	s.record(obs, len(offered), o, err, time.Since(start))
	return o, err
}

func (s *statsPlanner) goalDone(obs agent.Observation) (bool, error) {
	status, structured, err := agent.PlannerGoalStatus(s.inner.Goal, obs)
	if err != nil {
		return false, err
	}
	return structured && status.Complete, nil
}

func (s *statsPlanner) prepareStrategy(obs agent.Observation) {
	// NextRetry receives the same observation and round as Next. Count a
	// world state once, not once per model attempt.
	if !s.strategySeen || obs.Round != s.strategyRound {
		s.strategy.ObserveProgress(obs, 0)
		s.strategyRound = obs.Round
		s.strategySeen = true
	}

	extra := s.baseExtraSystem
	if reason := s.strategy.ReplanReason(strategicReplanAfter, obs.Intent); reason != "" {
		if extra != "" {
			extra += "\n\n"
		}
		extra += "RUN REPLAN SIGNAL: " + reason
	}
	s.inner.ExtraSystem = extra
}

// record folds one ask into the tally and publishes it. A re-ask after a
// rejection counts as a call and as a rejection, never as a round: the
// round is the same one, asked again.
func (s *statsPlanner) record(obs agent.Observation, offered int, o agent.Objective, err error, took time.Duration) {
	s.stats.Calls++
	s.offered += offered
	s.elapsed += took
	s.stats.LastSeconds = took.Seconds()
	s.stats.AvgOffered = float64(s.offered) / float64(s.stats.Calls)
	s.stats.AvgSeconds = s.elapsed.Seconds() / float64(s.stats.Calls)
	s.stats.Round, s.stats.RoundsLeft = obs.Round, obs.RoundsLeft
	s.stats.Intent, s.stats.IntentAge = obs.Intent, obs.IntentAge

	h := s.inner.Health
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
	if s.snap != nil {
		s.snap.storeStats(s.stats)
	}
	if s.push != nil {
		s.push(s.stats)
	}
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
