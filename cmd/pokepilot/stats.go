package main

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
)

type (
	runStats    = farm.LLMStats
	choiceCount = farm.ChoiceCount
)

const strategicReplanAfter = 4

type statsPlanner struct {
	inner  *agent.LLMPlanner
	router *agent.FailoverPlanner

	emu  *emu.Emu
	push func(any)
	snap *heartbeatSnap

	// llm_profile chooses inference routing; these fields choose orthogonal
	// gameplay policy. They intentionally remain independent knobs.
	playStyle      agent.PlayStyleProfile
	riskTolerance  string
	wildEncounters string

	stats                runStats
	counts               map[string]int
	offered              int
	elapsed              time.Duration
	successfulCalls      int
	successfulElapsed    time.Duration
	rejectedElapsed      time.Duration
	seenPromptTokens     int
	seenCompletionTokens int
	lastTelemetrySeq     uint64

	runGoalStatus        agent.GoalStatus
	runGoalDeterministic bool

	strategy        agent.StrategicMemory
	strategyRound   int
	strategySeen    bool
	stallCaptured   bool
	baseExtraSystem string
}

// newStatsPlanner remains source-compatible with existing local/tests. Farm
// construction consumes run policy from the lease farm.Client just decoded;
// local construction consumes the corresponding CLI flags. Empty values keep
// the historical compatibility defaults.
func newStatsPlanner(profile, reasoningEffort, goal string, m *emu.Emu, push func(any), snap *heartbeatSnap) *statsPlanner {
	playStyle := localPlayStyleName()
	riskTolerance := localRiskToleranceName()
	wildEncounters := localWildEncountersName()
	if snap != nil {
		playStyle = farm.CurrentPlayStyle()
		riskTolerance = farm.CurrentRiskTolerance()
		wildEncounters = farm.CurrentWildEncounters()
	}
	return newStatsPlannerWithRunPolicy(profile, reasoningEffort, playStyle, riskTolerance, wildEncounters, goal, m, push, snap)
}

// newStatsPlannerWithPlayStyle is kept for existing tests/callers that only
// choose a play style. Empty risk/wild settings preserve the old behavior.
func newStatsPlannerWithPlayStyle(profile, reasoningEffort, playStyle, goal string, m *emu.Emu, push func(any), snap *heartbeatSnap) *statsPlanner {
	return newStatsPlannerWithRunPolicy(profile, reasoningEffort, playStyle, "", "", goal, m, push, snap)
}

func newStatsPlannerWithRunPolicy(profile, reasoningEffort, playStyle, riskTolerance, wildEncounters, goal string, m *emu.Emu, push func(any), snap *heartbeatSnap) *statsPlanner {
	primaryCfg, fallbackCfg := agent.ResolveLLMEndpointsWithEffort(agent.NormalizeLLMProfile(profile), agent.NormalizeReasoningEffort(reasoningEffort))
	inner := agent.NewLLMPlannerFromConfig(primaryCfg)
	inner.Goal = goal
	var fallback *agent.LLMPlanner
	if fallbackCfg != nil {
		fallback = agent.NewLLMPlannerFromConfig(*fallbackCfg)
	}

	s := &statsPlanner{
		inner:            inner,
		emu:              m,
		push:             push,
		snap:             snap,
		playStyle:        agent.PlayStyle(playStyle),
		riskTolerance:    agent.NormalizeRiskTolerance(riskTolerance),
		wildEncounters:   agent.NormalizeWildEncounters(wildEncounters),
		counts:           map[string]int{},
		baseExtraSystem:  inner.ExtraSystem,
		lastTelemetrySeq: currentLLMTelemetrySeq(),
	}
	s.router = agent.NewFailoverPlanner(inner, fallback)
	s.router.OnCall = s.recordCall
	return s
}

func (s *statsPlanner) wirePlannerLogs(log io.Writer, snap *heartbeatSnap) {
	if log != nil {
		s.inner.Log = log
	}
	if snap != nil {
		s.inner.PromptLog = rawWriter{snap: snap, start: true}
		s.inner.ReplyLog = rawWriter{snap: snap}
	}
}

func (s *statsPlanner) Next(obs agent.Observation, offered []agent.Objective) (agent.Objective, error) {
	s.prepareRunContext(obs)
	return s.ask(obs, offered, nil)
}

func (s *statsPlanner) NextRetry(obs agent.Observation, offered []agent.Objective, r agent.Retry) (agent.Objective, error) {
	s.prepareRunContext(obs)
	return s.ask(obs, offered, &r)
}

func (s *statsPlanner) applyRunPolicy(obs agent.Observation, offered []agent.Objective) []agent.Objective {
	offered = agent.ApplyRunPolicy(obs, offered, s.riskTolerance, s.wildEncounters)
	return agent.AnnotatePlayStyle(obs, offered, s.playStyle)
}

// boundRiskPlan keeps persistent planning from skipping the safety decision
// that only becomes knowable after a battle or a fight-through travel leg.
// A Balanced/Cautious plan may still batch harmless navigation/interactions,
// but it stops at the first objective that can materially change party health;
// the next round observes the real post-action HP/PP before committing again.
func (s *statsPlanner) boundRiskPlan(plan agent.Plan, offered []agent.Objective) agent.Plan {
	if s.riskTolerance == agent.RiskToleranceAggressive || len(plan.Steps) <= 1 {
		return plan
	}
	for i, step := range plan.Steps {
		o, err := agent.Chosen(offered, step)
		if err != nil {
			continue
		}
		risky := false
		switch o.Kind {
		case agent.KindTrainer, agent.KindGym, agent.KindTrain, agent.KindCatch:
			risky = true
		case agent.KindGoTo:
			// Plain travel fights wild encounters. Fight-everything also turns
			// any originally-fleeing journey into that form before planning.
			risky = !o.Flee || s.wildEncounters == agent.WildEncountersFight
		}
		if !risky {
			continue
		}
		if i+1 < len(plan.Steps) {
			plan.Steps = append([]string(nil), plan.Steps[:i+1]...)
		}
		break
	}
	return plan
}

func (s *statsPlanner) ask(obs agent.Observation, offered []agent.Objective, retry *agent.Retry) (agent.Objective, error) {
	if s.snap != nil {
		offered = farmRecoveryOffered(obs, offered)
	}
	offered = s.applyRunPolicy(obs, offered)
	if retry == nil {
		return s.router.Next(obs, offered)
	}
	return s.router.NextRetry(obs, offered, *retry)
}

func (s *statsPlanner) Strategize(obs agent.Observation, offered []agent.Objective, reason string) (agent.Plan, error) {
	s.prepareRunContext(obs)
	offered = s.applyRunPolicy(obs, offered)
	plan, err := s.router.Strategize(obs, offered, reason)
	if err != nil {
		return plan, err
	}
	return s.boundRiskPlan(plan, offered), nil
}

func (s *statsPlanner) StrategizeRetry(obs agent.Observation, offered []agent.Objective, reason string, r agent.Retry) (agent.Plan, error) {
	s.prepareRunContext(obs)
	offered = s.applyRunPolicy(obs, offered)
	plan, err := s.router.StrategizeRetry(obs, offered, reason, r)
	if err != nil {
		return plan, err
	}
	return s.boundRiskPlan(plan, offered), nil
}

func (s *statsPlanner) ObservePlanning(p agent.PlanningStats) {
	s.stats.PlanGoal = p.Plan.Goal
	s.stats.PlanSteps = append([]string(nil), p.Plan.Steps...)
	s.stats.PlanStep = p.Plan.Step
	s.stats.PlanRound = p.Plan.Round
	s.stats.PlanExecutions = p.PlanExecutions
	s.stats.StepsSkipped = p.StepsSkipped
	s.stats.LastReplanReason = p.LastReplanReason
	s.stats.ReplanReasons = make(map[string]int, len(p.ReplanReasons))
	for k, v := range p.ReplanReasons {
		s.stats.ReplanReasons[k] = v
	}
	s.publish()
}

func (s *statsPlanner) prepareRunContext(obs agent.Observation) {
	s.prepareStrategyWithGoal(obs, s.runGoalStatus, s.runGoalDeterministic)
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

func (s *statsPlanner) record(obs agent.Observation, offered int, o agent.Objective, err error, took time.Duration) {
	s.recordCall(agent.LLMCall{Observation: obs, Offered: offered, Objective: o, Err: err, Duration: took})
}

func (s *statsPlanner) recordCall(call agent.LLMCall) {
	obs, offered, o, err, took := call.Observation, call.Offered, call.Objective, call.Err, call.Duration
	s.stats.Calls++
	s.offered += offered
	s.elapsed += took
	s.stats.LastSeconds = took.Seconds()
	s.stats.AvgOffered = float64(s.offered) / float64(s.stats.Calls)
	s.stats.AvgSeconds = s.elapsed.Seconds() / float64(s.stats.Calls)
	if err != nil {
		s.stats.Rejected++
		s.rejectedElapsed += took
		s.stats.RejectedAvgSeconds = s.rejectedElapsed.Seconds() / float64(s.stats.Rejected)
	} else {
		s.successfulCalls++
		s.successfulElapsed += took
		s.stats.SuccessfulAvgSeconds = s.successfulElapsed.Seconds() / float64(s.successfulCalls)
	}
	s.stats.Round, s.stats.RoundsLeft = obs.Round, obs.RoundsLeft
	s.stats.Intent, s.stats.IntentAge = obs.Intent, obs.IntentAge
	route := s.router.Route()
	s.stats.Backend, s.stats.Model, s.stats.Failovers = route.Backend, route.Model, route.Failovers

	h := s.router.Health()
	s.stats.PromptTokens, s.stats.CompletionTokens = h.PromptTokens, h.CompletionTokens
	s.stats.Transport, s.stats.Fallbacks = h.Transport, h.Fallbacks
	s.stats.LastPromptTokens = h.PromptTokens - s.seenPromptTokens
	s.stats.LastCompletionTokens = h.CompletionTokens - s.seenCompletionTokens
	if s.stats.LastPromptTokens < 0 {
		s.stats.LastPromptTokens = 0
	}
	if s.stats.LastCompletionTokens < 0 {
		s.stats.LastCompletionTokens = 0
	}
	s.seenPromptTokens, s.seenCompletionTokens = h.PromptTokens, h.CompletionTokens

	if telemetry, ok := latestLLMTelemetryAfter(s.lastTelemetrySeq); ok {
		s.lastTelemetrySeq = telemetry.Seq
		s.stats.Endpoint = telemetry.Endpoint
		s.stats.ResponseModel = telemetry.ResponseModel
		if telemetry.PromptTokens > 0 {
			s.stats.LastPromptTokens = telemetry.PromptTokens
		}
		if telemetry.CompletionTokens > 0 {
			s.stats.LastCompletionTokens = telemetry.CompletionTokens
		}
		s.stats.LastCachedPromptTokens = telemetry.CachedPromptTokens
		s.stats.PrefillMS = telemetry.PrefillMS
		s.stats.PrefillTPS = telemetry.PrefillTPS
		s.stats.DecodeMS = telemetry.DecodeMS
		s.stats.DecodeTPS = telemetry.DecodeTPS
		s.stats.OverheadMS = telemetry.OverheadMS
		s.stats.TimingSource = telemetry.TimingSource
	}

	if call.Strategic {
		s.stats.StrategicCalls++
		s.stats.StrategicSeconds += took.Seconds()
		s.stats.StrategicAvgSeconds = s.stats.StrategicSeconds / float64(s.stats.StrategicCalls)
		s.publish()
		return
	}

	s.stats.FastCalls++
	if err == nil {
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

func (s *statsPlanner) Usage() (prompt, completion int) {
	return s.router.Usage()
}

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
