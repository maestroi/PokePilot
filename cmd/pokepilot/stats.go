package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	decision       agent.DecisionSettings

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
	if inference := farm.CurrentInference(); inference != nil && inference.Endpoint != "" && inference.APIModel != "" {
		// A first-class leased deployment is authoritative. Keep the endpoint
		// tuning defaults (no-think, token budget, reasoning effort), but route
		// the request to the exact endpoint/model identity the wall resolved.
		// This is what lets a pinned llama.cpp endpoint change 27B -> 9B without
		// rebuilding the runner, and prevents experiment identity from diverging
		// from the model actually requested.
		primaryCfg.BaseURL = inference.Endpoint
		primaryCfg.Model = inference.APIModel
		primaryCfg.Token = ""
		if inference.TokenEnv != "" {
			primaryCfg.Token = os.Getenv(inference.TokenEnv)
		}
		fallbackCfg = nil
	}
	inner := agent.NewLLMPlannerFromConfig(primaryCfg)
	inner.Goal = goal
	inner.OnModelAdopted = farm.AdoptCurrentInferenceModel
	var fallback *agent.LLMPlanner
	if fallbackCfg != nil {
		fallback = agent.NewLLMPlannerFromConfig(*fallbackCfg)
		fallback.OnModelAdopted = farm.AdoptCurrentInferenceModel
	}

	s := &statsPlanner{
		inner:            inner,
		emu:              m,
		push:             push,
		snap:             snap,
		playStyle:        agent.PlayStyle(playStyle),
		riskTolerance:    agent.NormalizeRiskTolerance(riskTolerance),
		wildEncounters:   agent.NormalizeWildEncounters(wildEncounters),
		decision:         agent.DecisionSettingsFromEnv(),
		counts:           map[string]int{},
		baseExtraSystem:  appendSystemNote(inner.ExtraSystem, agent.PlayStyleSystemNote(playStyle)),
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
		s.inner.PromptLog = policyRawWriter{
			snap:           snap,
			playStyle:      s.playStyle.Name,
			riskTolerance:  s.riskTolerance,
			wildEncounters: s.wildEncounters,
		}
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

func (s *statsPlanner) RoutePriority() agent.RoutePriority {
	return agent.RoutePriorityForPlayStyle(s.playStyle)
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
	if retry == nil && s.decision.Engine != nil && s.decision.ObjectiveSelection {
		if objective, ok := s.typedObjective(obs, offered); ok {
			return objective, nil
		}
	}
	if retry == nil {
		return s.router.Next(obs, offered)
	}
	return s.router.NextRetry(obs, offered, *retry)
}

func (s *statsPlanner) typedObjective(obs agent.Observation, offered []agent.Objective) (agent.Objective, bool) {
	req, err := agent.ObjectiveDecisionRequest(obs, offered, s.inner.Goal)
	if err != nil {
		s.recordDecision(req, agent.DecisionResponse{}, err, true)
		return agent.Objective{}, false
	}
	resp, err := agent.DecideChecked(context.Background(), s.decision.Engine, req)
	if err == nil && s.decision.MinConfidence > 0 && resp.Confidence < s.decision.MinConfidence {
		err = fmt.Errorf("%w: %.3f < %.3f", agent.ErrDecisionLowConfidence, resp.Confidence, s.decision.MinConfidence)
	}
	var objective agent.Objective
	if err == nil {
		objective, err = agent.Chosen(offered, resp.Choice)
	}
	fallback := err != nil
	s.recordDecision(req, resp, err, fallback)
	if err != nil {
		return agent.Objective{}, false
	}
	s.recordTypedObjectiveChoice(obs, objective)
	return objective, true
}

// DecideFailure implements agent.FailureDecisionPlanner. Run calls it only for
// failures already admitted by deterministic recovery policy; an unavailable,
// rejected, or low-confidence typed answer simply falls back to that existing
// policy.
func (s *statsPlanner) DecideFailure(result agent.ObjectiveResult) (agent.DecisionResponse, error) {
	if s.decision.Engine == nil || !s.decision.FailureRecovery {
		return agent.DecisionResponse{}, agent.ErrDecisionDisabled
	}
	req, err := agent.FailureDecisionRequest(result)
	if err != nil {
		s.recordDecision(req, agent.DecisionResponse{}, err, true)
		return agent.DecisionResponse{}, err
	}
	resp, err := agent.DecideChecked(context.Background(), s.decision.Engine, req)
	if err == nil && s.decision.MinConfidence > 0 && resp.Confidence < s.decision.MinConfidence {
		err = fmt.Errorf("%w: %.3f < %.3f", agent.ErrDecisionLowConfidence, resp.Confidence, s.decision.MinConfidence)
	}
	s.recordDecision(req, resp, err, err != nil)
	return resp, err
}

func (s *statsPlanner) recordTypedObjectiveChoice(obs agent.Observation, objective agent.Objective) {
	s.stats.FastCalls++
	s.stats.Rounds++
	s.stats.Round, s.stats.RoundsLeft = obs.Round, obs.RoundsLeft
	name := objective.String()
	if s.counts[name] > 0 {
		s.stats.Repeats++
	}
	s.counts[name]++
	s.stats.Choices = rankChoices(s.counts)
	s.publish()
}

func cloneDecisionProbabilities(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (s *statsPlanner) recordDecision(req agent.DecisionRequest, resp agent.DecisionResponse, err error, fallback bool) {
	s.stats.DecisionCalls++
	s.stats.DecisionSeconds += resp.Duration.Seconds()
	s.stats.DecisionAvgSeconds = s.stats.DecisionSeconds / float64(s.stats.DecisionCalls)
	s.stats.DecisionPromptTokens += resp.Usage.PromptTokens
	s.stats.DecisionCompletionTokens += resp.Usage.CompletionTokens
	s.stats.DecisionInputBytes += resp.Usage.InputBytes
	s.stats.DecisionOutputBytes += resp.Usage.OutputBytes
	if err != nil {
		s.stats.DecisionRejected++
	}
	if fallback {
		s.stats.DecisionFallbacks++
	}
	if resp.Backend != "" {
		s.stats.DecisionBackend = resp.Backend
	} else if s.decision.Backend != "" {
		s.stats.DecisionBackend = s.decision.Backend
	}
	if resp.Model != "" {
		s.stats.DecisionModel = resp.Model
	}
	s.stats.DecisionKind = req.Kind
	s.stats.DecisionChoice = resp.Choice
	s.stats.DecisionConfidence = resp.Confidence
	s.stats.DecisionProbabilities = cloneDecisionProbabilities(resp.Probabilities)

	record := farm.TypedDecisionRecord{
		Kind:             req.Kind,
		Question:         req.Question,
		Choice:           resp.Choice,
		Probabilities:    cloneDecisionProbabilities(resp.Probabilities),
		Confidence:       resp.Confidence,
		DurationSeconds:  resp.Duration.Seconds(),
		Backend:          resp.Backend,
		Model:            resp.Model,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		InputBytes:       resp.Usage.InputBytes,
		OutputBytes:      resp.Usage.OutputBytes,
		Fallback:         fallback,
	}
	if record.Backend == "" {
		record.Backend = s.decision.Backend
	}
	if err != nil {
		record.Error = err.Error()
	}
	const maxDecisionRecords = 128
	if len(s.stats.DecisionRecords) < maxDecisionRecords {
		s.stats.DecisionRecords = append(s.stats.DecisionRecords, record)
	} else {
		s.stats.DecisionRecordsDropped++
	}
	s.publish()
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
		observation, _ := json.Marshal(obs)
		record := farm.StrategicCallRecord{
			Observation:      observation,
			Offered:          append([]string(nil), call.OfferedObjectives...),
			ReplanReason:     call.ReplanReason,
			PlanGoal:         call.Plan.Goal,
			PlanSteps:        append([]string(nil), call.Plan.Steps...),
			Rejected:         err != nil,
			DurationSeconds:  took.Seconds(),
			Backend:          s.stats.Backend,
			Model:            s.stats.Model,
			PromptTokens:     s.stats.LastPromptTokens,
			CompletionTokens: s.stats.LastCompletionTokens,
			PrefillTPS:       s.stats.PrefillTPS,
			DecodeTPS:        s.stats.DecodeTPS,
		}
		if err != nil {
			record.Error = err.Error()
		}
		const maxStrategicRecords = 64
		if len(s.stats.StrategicRecords) < maxStrategicRecords {
			s.stats.StrategicRecords = append(s.stats.StrategicRecords, record)
		} else {
			s.stats.StrategicRecordsDropped++
		}
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
