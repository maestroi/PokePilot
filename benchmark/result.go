package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

const ResultVersion = 1

type MilestoneDefinition struct {
	ID    string
	Name  string
	Goal  string
	Match func(agent.Observation) bool
}

func (m MilestoneDefinition) Reached(obs agent.Observation) bool {
	if m.Match != nil && m.Match(obs) {
		return true
	}
	if m.Goal == "" {
		return false
	}
	status, structured, err := agent.PlannerGoalStatus(m.Goal, obs)
	return err == nil && structured && status.Complete
}

type Profile struct {
	Game       string
	Milestones []MilestoneDefinition
}

func (p Profile) Milestone(id string) (MilestoneDefinition, bool) {
	id = normalizeID(id)
	for _, milestone := range p.Milestones {
		if milestone.ID == id {
			return milestone, true
		}
	}
	return MilestoneDefinition{}, false
}

func (p Profile) GoalFor(id string) (string, bool) {
	milestone, ok := p.Milestone(id)
	return milestone.Goal, ok && milestone.Goal != ""
}

type Source struct {
	Kind             string `json:"kind"`
	Checkpoint       string `json:"checkpoint,omitempty"`
	CheckpointSHA256 string `json:"checkpoint_sha256,omitempty"`
	OriginRunID      string `json:"origin_run_id,omitempty"`
	OriginCommit     string `json:"origin_commit,omitempty"`
	OriginMilestone  string `json:"origin_milestone,omitempty"`
	OriginSeed       int64  `json:"origin_seed,omitempty"`
}

type ModelIdentity struct {
	Profile                 string `json:"profile,omitempty"`
	PrimaryModel            string `json:"primary_model,omitempty"`
	PrimaryURL              string `json:"primary_url,omitempty"`
	NoThink                 bool   `json:"no_think,omitempty"`
	MaxTokens               int    `json:"max_tokens,omitempty"`
	Timeout                 string `json:"timeout,omitempty"`
	ReasoningEffort         string `json:"reasoning_effort,omitempty"`
	RecoveryReasoningEffort string `json:"recovery_reasoning_effort,omitempty"`
	PromptHash              string `json:"prompt_hash,omitempty"`
	FallbackModel           string `json:"fallback_model,omitempty"`
	FallbackURL             string `json:"fallback_url,omitempty"`
	FallbackNoThink         bool   `json:"fallback_no_think,omitempty"`
	FallbackMaxTokens       int    `json:"fallback_max_tokens,omitempty"`
	FallbackTimeout         string `json:"fallback_timeout,omitempty"`
	FallbackReasoningEffort string `json:"fallback_reasoning_effort,omitempty"`
}

type Configuration struct {
	Planner         string            `json:"planner,omitempty"`
	Goal            string            `json:"goal,omitempty"`
	Starter         string            `json:"starter,omitempty"`
	LLMProfile      string            `json:"llm_profile,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
	PlayStyle       string            `json:"play_style,omitempty"`
	Purpose         string            `json:"purpose,omitempty"`
	RiskTolerance   string            `json:"risk_tolerance,omitempty"`
	WildEncounters  string            `json:"wild_encounters,omitempty"`
	DecisionBackend string            `json:"decision_backend,omitempty"`
	EmulatorSpeed   string            `json:"emulator_speed,omitempty"`
	MaxFrames       int               `json:"max_frames,omitempty"`
	Model           ModelIdentity     `json:"model"`
	FeatureFlags    map[string]string `json:"feature_flags,omitempty"`
}

type PartyMember struct {
	Species string `json:"species"`
	Level   uint8  `json:"level"`
	HP      uint16 `json:"hp"`
	MaxHP   uint16 `json:"max_hp"`
	Status  string `json:"status,omitempty"`
}

type Capability struct {
	Name       string `json:"name"`
	BadgeOwned bool   `json:"badge_owned,omitempty"`
	HMOwned    bool   `json:"hm_owned,omitempty"`
	Learned    bool   `json:"learned,omitempty"`
	Usable     bool   `json:"usable,omitempty"`
}

type Split struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	Round               int           `json:"round,omitempty"`
	Frame               uint64        `json:"frame"`
	FramesSincePrevious uint64        `json:"frames_since_previous"`
	WallSeconds         float64       `json:"wall_seconds"`
	WallSincePrevious   float64       `json:"wall_since_previous"`
	Map                 string        `json:"map,omitempty"`
	X                   uint8         `json:"x"`
	Y                   uint8         `json:"y"`
	Party               []PartyMember `json:"party,omitempty"`
	Badges              []string      `json:"badges,omitempty"`
	Capabilities        []Capability  `json:"capabilities,omitempty"`
	Objective           string        `json:"objective,omitempty"`
	Checkpoint          string        `json:"checkpoint,omitempty"`
}

type TimingBucket struct {
	Frames      uint64  `json:"frames,omitempty"`
	WallSeconds float64 `json:"wall_seconds,omitempty"`
	Count       int64   `json:"count,omitempty"`
}

type DecisionCall struct {
	Kind             string
	Duration         time.Duration
	PromptTokens     int
	CompletionTokens int
	Backend          string
	Model            string
	Err              error
}

type DecisionStats struct {
	Calls            int     `json:"calls,omitempty"`
	Failures         int     `json:"failures,omitempty"`
	TotalLatencySec  float64 `json:"total_latency_seconds,omitempty"`
	P50LatencySec    float64 `json:"p50_latency_seconds,omitempty"`
	P95LatencySec    float64 `json:"p95_latency_seconds,omitempty"`
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	Backend          string  `json:"backend,omitempty"`
	Model            string  `json:"model,omitempty"`
}

type ModelStats struct {
	Calls            int             `json:"calls"`
	StrategistCalls  int             `json:"strategist_calls,omitempty"`
	FastCalls        int             `json:"fast_calls,omitempty"`
	Failures         int             `json:"failures,omitempty"`
	TotalLatencySec  float64         `json:"total_latency_seconds,omitempty"`
	P50LatencySec    float64         `json:"p50_latency_seconds,omitempty"`
	P95LatencySec    float64         `json:"p95_latency_seconds,omitempty"`
	PromptTokens     int             `json:"prompt_tokens,omitempty"`
	CompletionTokens int             `json:"completion_tokens,omitempty"`
	Route            agent.LLMRoute  `json:"route"`
	Health           agent.LLMHealth `json:"health"`
}

type HistoryEntry struct {
	Round     int    `json:"round"`
	Objective string `json:"objective"`
	Outcome   string `json:"outcome"`
	Summary   string `json:"summary,omitempty"`
	Frame     uint64 `json:"frame,omitempty"`
}

type Failure struct {
	Key            string                `json:"key,omitempty"`
	Fingerprint    string                `json:"fingerprint,omitempty"`
	Round          int                   `json:"round,omitempty"`
	Frame          uint64                `json:"frame,omitempty"`
	Objective      string                `json:"objective,omitempty"`
	Outcome        string                `json:"outcome,omitempty"`
	Cause          string                `json:"cause,omitempty"`
	ErrorChain     []string              `json:"error_chain,omitempty"`
	Summary        string                `json:"summary,omitempty"`
	Map            string                `json:"map,omitempty"`
	X              uint8                 `json:"x"`
	Y              uint8                 `json:"y"`
	Checkpoint     string                `json:"checkpoint,omitempty"`
	Semantic       agent.FailureState    `json:"semantic_state"`
	Recent         []HistoryEntry        `json:"recent_history,omitempty"`
	RecentEvents   []string              `json:"recent_events,omitempty"`
	Travel         *agent.TravelEvidence `json:"travel,omitempty"`
	RouteBlockages []agent.RouteBlockage `json:"route_blockages,omitempty"`
	Planning       agent.PlanningStats   `json:"planning"`
	Reproduce      string                `json:"reproduce,omitempty"`
}

type Result struct {
	Version         int                     `json:"version"`
	RunID           string                  `json:"run_id"`
	Commit          string                  `json:"commit,omitempty"`
	StartedAt       time.Time               `json:"started_at"`
	FinishedAt      time.Time               `json:"finished_at"`
	Game            string                  `json:"game"`
	ROMSHA256       string                  `json:"rom_sha256"`
	Mode            string                  `json:"mode"`
	Seed            int64                   `json:"seed"`
	Source          Source                  `json:"source"`
	Configuration   Configuration           `json:"configuration"`
	Outcome         string                  `json:"outcome"`
	EndCondition    string                  `json:"end_condition"`
	Stop            string                  `json:"stop"`
	Frames          uint64                  `json:"frames"`
	EmulatorCycles  *uint64                 `json:"emulator_cycles,omitempty"`
	EmulatedSeconds float64                 `json:"emulated_seconds"`
	WallSeconds     float64                 `json:"wall_seconds"`
	Milestones      []Split                 `json:"milestones"`
	Timing          map[string]TimingBucket `json:"timing"`
	Counters        map[string]int64        `json:"counters"`
	Model           ModelStats              `json:"model"`
	Decision        DecisionStats           `json:"decision,omitempty"`
	Failures        []Failure               `json:"failures,omitempty"`
	LastMilestone   string                  `json:"last_milestone,omitempty"`
	ActiveObjective string                  `json:"active_objective,omitempty"`
	ExperimentID    string                  `json:"experiment_id,omitempty"`
	ExperimentArm   string                  `json:"experiment_arm,omitempty"`
	ExperimentCase  string                  `json:"experiment_case,omitempty"`
}

type CheckpointMetadata struct {
	Version       int           `json:"version"`
	RunID         string        `json:"run_id"`
	Commit        string        `json:"commit,omitempty"`
	Game          string        `json:"game"`
	ROMSHA256     string        `json:"rom_sha256"`
	Seed          int64         `json:"seed"`
	Milestone     string        `json:"milestone"`
	Frame         uint64        `json:"frame"`
	Configuration Configuration `json:"configuration"`
}

type BuildInput struct {
	RunID         string
	Commit        string
	Game          string
	ROMSHA256     string
	Mode          string
	Seed          int64
	Source        Source
	EndCondition  string
	Configuration Configuration
	Profile       Profile
	AgentResult   agent.Result
	Calls         []agent.LLMCall
	DecisionCalls []DecisionCall
	Route         agent.LLMRoute
	Health        agent.LLMHealth
	StartedAt     time.Time
	FinishedAt    time.Time
	TerminalError error
}

func Build(in BuildInput) Result {
	res := in.AgentResult
	frames := delta(res.FinalFrame, res.StartFrame)
	out := Result{
		Version:         ResultVersion,
		RunID:           in.RunID,
		Commit:          in.Commit,
		StartedAt:       in.StartedAt.UTC(),
		FinishedAt:      in.FinishedAt.UTC(),
		Game:            in.Game,
		ROMSHA256:       in.ROMSHA256,
		Mode:            in.Mode,
		Seed:            in.Seed,
		Source:          in.Source,
		Configuration:   in.Configuration,
		Outcome:         "failed",
		EndCondition:    in.EndCondition,
		Stop:            StopName(res.Stop),
		Frames:          frames,
		EmulatedSeconds: float64(frames) / farm.GameBoyFramesPerSecond,
		WallSeconds:     in.FinishedAt.Sub(in.StartedAt).Seconds(),
		Timing:          timing(res, in.Calls, in.DecisionCalls),
		Counters:        counters(res),
		Model:           modelStats(res, in.Calls, in.Route, in.Health),
		Decision:        decisionStats(in.DecisionCalls),
	}
	if len(in.DecisionCalls) > 0 {
		out.Counters["typed_decision_calls"] = int64(len(in.DecisionCalls))
		for _, call := range in.DecisionCalls {
			if call.Err != nil {
				out.Counters["typed_decision_failures"]++
			}
		}
	}
	if res.Stop == agent.StopDone && (res.GoalStatus == nil || res.GoalStatus.Complete) {
		out.Outcome = "completed"
	}
	out.Milestones = splits(in.Profile, in.Source, res)
	if len(out.Milestones) > 0 {
		out.LastMilestone = out.Milestones[len(out.Milestones)-1].ID
	}
	if len(res.Outcomes) > 0 {
		out.ActiveObjective = res.Outcomes[len(res.Outcomes)-1].Objective.String()
	}
	if out.Outcome != "completed" {
		if failure, ok := buildFailure(in); ok {
			out.Failures = []Failure{failure}
		}
	}
	return out
}

func StopName(s agent.Stop) string {
	switch s {
	case agent.StopDone:
		return "done"
	case agent.StopStuck:
		return "stuck"
	case agent.StopBudget:
		return "budget"
	case agent.StopFailed:
		return "failed"
	case agent.StopError:
		return "error"
	default:
		return "unset"
	}
}

func splits(profile Profile, source Source, res agent.Result) []Split {
	startID, startName := "fresh_start", "Fresh controllable start"
	if source.Kind == "checkpoint" {
		startID, startName = "checkpoint_start", "Checkpoint start"
	}
	out := []Split{splitFromObservation(startID, startName, 0, res.StartFrame, 0, 0, 0, res.Initial, "")}
	initial := map[string]bool{}
	for _, definition := range profile.Milestones {
		initial[definition.ID] = definition.Reached(res.Initial)
	}
	seen := map[string]bool{}
	prevFrame := res.StartFrame
	prevWall := 0.0
	for i, outcome := range res.Outcomes {
		if i >= len(res.OutcomeTimings) {
			break
		}
		t := res.OutcomeTimings[i]
		wall := t.WallElapsed.Seconds()
		for _, definition := range profile.Milestones {
			if seen[definition.ID] || initial[definition.ID] || !definition.Reached(outcome.Final) {
				continue
			}
			s := splitFromObservation(definition.ID, definition.Name, t.Round, t.Frame, delta(t.Frame, prevFrame), wall, wall-prevWall, outcome.Final, outcome.Objective.String())
			out = append(out, s)
			seen[definition.ID] = true
			prevFrame = t.Frame
			prevWall = wall
		}
	}
	return out
}

func splitFromObservation(id, name string, round int, frame, frameDelta uint64, wall, wallDelta float64, obs agent.Observation, objective string) Split {
	s := Split{
		ID: id, Name: name, Round: round, Frame: frame, FramesSincePrevious: frameDelta,
		WallSeconds: wall, WallSincePrevious: math.Max(0, wallDelta), Map: obs.MapName,
		X: obs.X, Y: obs.Y, Badges: append([]string(nil), obs.Badges...), Objective: objective,
	}
	for _, mon := range obs.Party {
		s.Party = append(s.Party, PartyMember{Species: string(mon.Species), Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP, Status: mon.Status})
	}
	for _, capability := range obs.FieldCapabilities {
		s.Capabilities = append(s.Capabilities, Capability{
			Name: string(capability.Name), BadgeOwned: capability.BadgeOwned, HMOwned: capability.HMOwned,
			Learned: capability.Learned, Usable: capability.Usable,
		})
	}
	return s
}

func timing(res agent.Result, calls []agent.LLMCall, decisionCalls []DecisionCall) map[string]TimingBucket {
	out := map[string]TimingBucket{}
	prev := res.StartFrame
	for i, result := range res.Outcomes {
		if i >= len(res.OutcomeTimings) {
			break
		}
		frame := res.OutcomeTimings[i].Frame
		name := objectiveBucket(result.Objective)
		if result.Recovered || (result.Outcome != agent.OutcomeCompleted && !result.Terminal) {
			name = "recovery"
		}
		bucket := out[name]
		bucket.Frames += delta(frame, prev)
		bucket.Count++
		out[name] = bucket
		prev = frame
	}
	if res.FinalFrame > prev {
		bucket := out["other_unclassified"]
		bucket.Frames += res.FinalFrame - prev
		out["other_unclassified"] = bucket
	}
	for _, call := range calls {
		name := "fast_inference"
		if call.Strategic {
			name = "strategist_inference"
		}
		bucket := out[name]
		bucket.WallSeconds += call.Duration.Seconds()
		bucket.Count++
		out[name] = bucket
	}
	for _, call := range decisionCalls {
		bucket := out["typed_decision_inference"]
		bucket.WallSeconds += call.Duration.Seconds()
		bucket.Count++
		out["typed_decision_inference"] = bucket
	}
	if res.Planning.StrategicCalls > 0 {
		bucket := out["planner_replanning"]
		bucket.Count = int64(sumMap(res.Planning.ReplanReasons))
		out["planner_replanning"] = bucket
	}
	return out
}

func objectiveBucket(o agent.Objective) string {
	switch o.Kind {
	case agent.KindGoTo:
		return "navigation"
	case agent.KindGym, agent.KindTrainer, agent.KindTrain, agent.KindCatch:
		return "battle"
	case agent.KindHeal:
		return "healing"
	case agent.KindBuy:
		return "shopping_inventory"
	case agent.KindUseItem, agent.KindPickup, agent.KindRepairFieldCapability:
		return "field_actions"
	case agent.KindTalk, agent.KindStarter, agent.KindProgress:
		return "menus"
	default:
		return "other_unclassified"
	}
}

func counters(res agent.Result) map[string]int64 {
	out := map[string]int64{
		"objectives_attempted":   int64(len(res.Outcomes)),
		"objectives_completed":   int64(len(res.Completed)),
		"objective_failures":     int64(len(res.Outcomes) - len(res.Completed)),
		"replans":                int64(sumMap(res.Planning.ReplanReasons)),
		"strategist_calls":       int64(res.Planning.StrategicCalls),
		"fast_planner_calls":     int64(res.Planning.FastCalls),
		"plan_executions":        int64(res.Planning.PlanExecutions),
		"leg_auto_executions":    int64(res.Planning.LegAutoExecutions),
		"leg_fast_executions":    int64(res.Planning.LegFastExecutions),
		"leg_boundaries":         int64(res.Planning.LegBoundaries),
		"leg_tail_steps_dropped": int64(res.Planning.LegTailStepsDropped),
		"planner_steps_skipped":  int64(res.Planning.StepsSkipped),
		"reply_retries":          int64(res.ReplyRetries),
	}
	previous := res.Initial
	for _, result := range res.Outcomes {
		if result.Recovered {
			out["successful_recoveries"]++
		}
		if result.Outcome != agent.OutcomeCompleted && !result.Terminal {
			out["recovery_attempts"]++
		}
		if result.Terminal {
			out["terminal_failures"]++
		}
		if objectiveBlackedOut(result) {
			out["blackouts"]++
		}
		if previous.Location != result.Final.Location || previous.MapName != result.Final.MapName {
			out["route_transitions"]++
		}
		if result.Travel != nil {
			out["battles"] += int64(result.Travel.Battles)
			out["wild_battles"] += int64(result.Travel.Battles)
			out["encounters"] += int64(result.Travel.Battles)
			out["fled_encounters"] += int64(result.Travel.Flees)
			out["local_navigation_replans"] += int64(result.Travel.Replans)
			out["emergency_navigation_egresses"] += int64(len(result.Travel.EmergencyEgresses))
		}
		if result.Train != nil {
			out["battles"] += int64(result.Train.Battles)
			out["wild_battles"] += int64(result.Train.Battles)
			out["encounters"] += int64(result.Train.Battles)
		}
		switch result.Objective.Kind {
		case agent.KindGym, agent.KindTrainer:
			out["battles"]++
			out["trainer_battles"]++
		case agent.KindCatch:
			out["battles"]++
			out["wild_battles"]++
			out["encounters"]++
		case agent.KindHeal:
			out["pokemon_center_visits"]++
		case agent.KindUseItem:
			item := strings.ToLower(string(result.Objective.Item))
			if strings.Contains(item, "repel") {
				out["repel_uses"]++
			} else if isHealingItem(item) {
				out["healing_item_uses"]++
			}
		}
		if result.Objective.RepelBeforeTravel {
			out["repel_uses"]++
		}
		previous = result.Final
	}
	return out
}

func modelStats(res agent.Result, calls []agent.LLMCall, route agent.LLMRoute, health agent.LLMHealth) ModelStats {
	latencies := make([]float64, 0, len(calls))
	promptTokens, completionTokens := res.PromptTokens, res.CompletionTokens
	if promptTokens == 0 && health.PromptTokens > 0 {
		promptTokens = health.PromptTokens
	}
	if completionTokens == 0 && health.CompletionTokens > 0 {
		completionTokens = health.CompletionTokens
	}
	stats := ModelStats{
		Calls: len(calls), PromptTokens: promptTokens, CompletionTokens: completionTokens,
		Route: route, Health: health,
	}
	for _, call := range calls {
		latencies = append(latencies, call.Duration.Seconds())
		stats.TotalLatencySec += call.Duration.Seconds()
		if call.Strategic {
			stats.StrategistCalls++
		} else {
			stats.FastCalls++
		}
		if call.Err != nil {
			stats.Failures++
		}
	}
	sort.Float64s(latencies)
	stats.P50LatencySec = percentile(latencies, 0.50)
	stats.P95LatencySec = percentile(latencies, 0.95)
	return stats
}

func decisionStats(calls []DecisionCall) DecisionStats {
	latencies := make([]float64, 0, len(calls))
	stats := DecisionStats{Calls: len(calls)}
	for _, call := range calls {
		latencies = append(latencies, call.Duration.Seconds())
		stats.TotalLatencySec += call.Duration.Seconds()
		stats.PromptTokens += call.PromptTokens
		stats.CompletionTokens += call.CompletionTokens
		if call.Err != nil {
			stats.Failures++
		}
		if call.Backend != "" {
			stats.Backend = call.Backend
		}
		if call.Model != "" {
			stats.Model = call.Model
		}
	}
	sort.Float64s(latencies)
	stats.P50LatencySec = percentile(latencies, 0.50)
	stats.P95LatencySec = percentile(latencies, 0.95)
	return stats
}

func buildFailure(in BuildInput) (Failure, bool) {
	res := in.AgentResult
	if len(res.Outcomes) == 0 {
		if in.TerminalError == nil && res.Err == nil {
			return Failure{}, false
		}
		return Failure{
			ErrorChain: errorChain(firstError(in.TerminalError, res.Err)),
			Summary:    firstError(in.TerminalError, res.Err).Error(),
			Map:        res.Final.MapName, X: res.Final.X, Y: res.Final.Y,
			Semantic: agent.FailureStateFor(res.Final), Planning: res.Planning,
		}, true
	}
	idx := len(res.Outcomes) - 1
	result := res.Outcomes[idx]
	timing := agent.ObjectiveTiming{}
	if idx < len(res.OutcomeTimings) {
		timing = res.OutcomeTimings[idx]
	}
	initial := agent.FailureState{}
	if result.Initial != nil {
		initial = *result.Initial
	}
	final := agent.FailureStateFor(result.Final)
	cause := string(result.Cause)
	identity := farm.FailureIdentity{
		Version:      farm.FailureIdentityVersion,
		Game:         "pokemon",
		Adapter:      in.Game,
		Objective:    farmObjective(agent.FailureObjectiveFor(result.Objective)),
		Outcome:      string(result.Outcome),
		Cause:        cause,
		CauseContext: append([]string(nil), result.CauseContext...),
		Initial:      farmState(initial),
		Final:        farmState(final),
	}
	var occurrence farm.FailureOccurrence
	var err error
	if cause != "" {
		occurrence, err = farm.NewFailureOccurrence(identity, in.Commit, timing.Round, "", result.Summary, in.FinishedAt)
	}
	failure := Failure{
		Round: timing.Round, Frame: timing.Frame, Objective: result.Objective.String(),
		Outcome: string(result.Outcome), Cause: cause, Summary: result.Summary,
		ErrorChain: errorChain(firstError(in.TerminalError, res.Err)),
		Map:        result.Final.MapName, X: result.Final.X, Y: result.Final.Y,
		Semantic: final, Recent: recentHistory(res, 6), RecentEvents: tailStrings(result.Final.Events, 12),
		Travel: result.Travel, RouteBlockages: append([]agent.RouteBlockage(nil), result.Final.RouteBlockages...), Planning: res.Planning,
	}
	if err == nil {
		failure.Key, failure.Fingerprint = occurrence.Key, occurrence.Fingerprint
	}
	return failure, true
}

func tailStrings(values []string, n int) []string {
	if len(values) <= n {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[len(values)-n:]...)
}

func recentHistory(res agent.Result, n int) []HistoryEntry {
	start := max(0, len(res.Outcomes)-n)
	out := make([]HistoryEntry, 0, len(res.Outcomes)-start)
	for i := start; i < len(res.Outcomes); i++ {
		result := res.Outcomes[i]
		entry := HistoryEntry{Objective: result.Objective.String(), Outcome: string(result.Outcome), Summary: result.Summary}
		if i < len(res.OutcomeTimings) {
			entry.Round = res.OutcomeTimings[i].Round
			entry.Frame = res.OutcomeTimings[i].Frame
		}
		out = append(out, entry)
	}
	return out
}

func MaterializeCheckpoints(result *Result, checkpointDir, outputDir, finalState string) error {
	for i := range result.Milestones {
		split := &result.Milestones[i]
		if split.Round == 0 {
			continue
		}
		source := checkpointForRound(checkpointDir, split.Round+1)
		if source == "" && finalState != "" {
			source = finalState
		}
		if source == "" {
			continue
		}
		dst := filepath.Join(outputDir, "checkpoints", split.ID+".state")
		if err := copyCheckpointFamily(source, dst); err != nil {
			return err
		}
		split.Checkpoint = filepath.ToSlash(filepath.Join("checkpoints", split.ID+".state"))
		if err := WriteJSON(checkpointMetadataPath(dst), checkpointMetadata(*result, split.ID, split.Frame)); err != nil {
			return err
		}
	}
	if len(result.Failures) > 0 {
		failure := &result.Failures[0]
		source := checkpointForRound(checkpointDir, failure.Round)
		if source != "" {
			dst := filepath.Join(outputDir, "checkpoints", "failure.state")
			if err := copyCheckpointFamily(source, dst); err != nil {
				return err
			}
			failure.Checkpoint = filepath.ToSlash(filepath.Join("checkpoints", "failure.state"))
			failure.Reproduce = fmt.Sprintf("pokebench red --from checkpoint:%s --until %s --runs 1", failure.Checkpoint, result.EndCondition)
			if err := WriteJSON(checkpointMetadataPath(dst), checkpointMetadata(*result, "failure", failure.Frame)); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkpointMetadata(result Result, milestone string, frame uint64) CheckpointMetadata {
	return CheckpointMetadata{
		Version: ResultVersion, RunID: result.RunID, Commit: result.Commit, Game: result.Game,
		ROMSHA256: result.ROMSHA256, Seed: result.Seed, Milestone: milestone, Frame: frame,
		Configuration: result.Configuration,
	}
}

func checkpointMetadataPath(statePath string) string {
	return strings.TrimSuffix(statePath, ".state") + ".benchmark.json"
}

func ReadCheckpointMetadata(statePath string) (CheckpointMetadata, bool, error) {
	data, err := os.ReadFile(checkpointMetadataPath(statePath))
	if errors.Is(err, os.ErrNotExist) {
		return CheckpointMetadata{}, false, nil
	}
	if err != nil {
		return CheckpointMetadata{}, false, err
	}
	var meta CheckpointMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return CheckpointMetadata{}, false, err
	}
	if meta.Version != ResultVersion {
		return CheckpointMetadata{}, false, fmt.Errorf("benchmark: checkpoint metadata version %d, want %d", meta.Version, ResultVersion)
	}
	return meta, true, nil
}

func checkpointForRound(dir string, round int) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	prefix := fmt.Sprintf("round-%03d-", round)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".state") {
			return filepath.Join(dir, entry.Name())
		}
	}
	return ""
}

func copyCheckpointFamily(srcState, dstState string) error {
	if err := os.MkdirAll(filepath.Dir(dstState), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(srcState)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dstState, data, 0o600); err != nil {
		return err
	}
	srcStem := strings.TrimSuffix(filepath.Base(srcState), ".state")
	dstStem := strings.TrimSuffix(filepath.Base(dstState), ".state")
	entries, err := os.ReadDir(filepath.Dir(srcState))
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), srcStem+".") {
			continue
		}
		suffix := strings.TrimPrefix(entry.Name(), srcStem)
		data, readErr := os.ReadFile(filepath.Join(filepath.Dir(srcState), entry.Name()))
		if readErr != nil {
			return readErr
		}
		if writeErr := os.WriteFile(filepath.Join(filepath.Dir(dstState), dstStem+suffix), data, 0o600); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func WriteJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func LoadResults(path string) ([]Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var paths []string
	if !info.IsDir() {
		paths = []string{path}
	} else {
		err = filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !d.IsDir() && strings.EqualFold(d.Name(), "benchmark-result.json") {
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var results []Result
	for _, candidate := range paths {
		data, readErr := os.ReadFile(candidate)
		if readErr != nil {
			return nil, readErr
		}
		var result Result
		if json.Unmarshal(data, &result) != nil || result.RunID == "" || result.Game == "" {
			continue
		}
		if result.Version != ResultVersion {
			return nil, fmt.Errorf("benchmark: %s has result version %d, want %d", candidate, result.Version, ResultVersion)
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("benchmark: no version-%d benchmark results in %s", ResultVersion, path)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].StartedAt.Before(results[j].StartedAt) })
	return results, nil
}

func SafeEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func SanitizeSettings(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		lower := strings.ToLower(key)
		if secretSettingKey(lower) {
			continue
		}
		if strings.Contains(lower, "url") || strings.Contains(lower, "endpoint") {
			value = SafeEndpoint(value)
			if value == "" {
				continue
			}
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func delta(end, start uint64) uint64 {
	if end <= start {
		return 0
	}
	return end - start
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(p*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func sumMap(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func isHealingItem(item string) bool {
	for _, token := range []string{"potion", "restore", "heal", "antidote", "awakening", "parlyz", "revive"} {
		if strings.Contains(item, token) {
			return true
		}
	}
	return false
}

func objectiveBlackedOut(result agent.ObjectiveResult) bool {
	return result.Final.BlackedOut ||
		(result.Travel != nil && result.Travel.BlackedOut) ||
		(result.Train != nil && result.Train.BlackedOut)
}

func errorChain(err error) []string {
	var out []string
	for err != nil {
		out = append(out, err.Error())
		err = errors.Unwrap(err)
	}
	return out
}

func firstError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return errors.New("benchmark failed without a terminal error")
}

func farmObjective(in agent.FailureObjective) farm.FailureObjective {
	return farm.FailureObjective{
		Kind: in.Kind, Place: string(in.Place), X: in.X, Y: in.Y, Starter: in.Starter,
		Progress: string(in.Progress), FieldCapability: string(in.FieldCapability), Level: in.Level,
		Species: string(in.Species), Item: string(in.Item), Slot: in.Slot, Qty: in.Qty, Flee: in.Flee,
	}
}

func farmState(in agent.FailureState) farm.FailureState {
	out := farm.FailureState{
		Location: string(in.Location), X: in.X, Y: in.Y, Controllable: in.Controllable,
		InBattle: in.InBattle, Money: in.Money, Badges: append([]string(nil), in.Badges...),
	}
	for _, mon := range in.Party {
		out.Party = append(out.Party, farm.FailurePartyMember{Species: string(mon.Species), Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP, Status: mon.Status})
	}
	for _, item := range in.Inventory {
		out.Inventory = append(out.Inventory, farm.FailureInventoryItem{ID: string(item.ID), Quantity: item.Quantity})
	}
	for _, capability := range in.Capabilities {
		out.Capabilities = append(out.Capabilities, farm.FailureCapability{
			ID: string(capability.ID), BadgeOwned: capability.BadgeOwned, HMOwned: capability.HMOwned,
			Learned: capability.Learned, Usable: capability.Usable,
		})
	}
	for _, fact := range in.Progress {
		out.Progress = append(out.Progress, farm.FailureProgressFact{ID: string(fact.ID), Complete: fact.Complete, Value: fact.Value})
	}
	return out
}

func secretSettingKey(lower string) bool {
	normalized := strings.NewReplacer("-", "_", ".", "_").Replace(strings.TrimSpace(lower))
	if normalized == "token" || strings.HasSuffix(normalized, "_token") {
		return true
	}
	return containsAny(normalized, "secret", "password", "credential", "authorization", "api_key", "apikey", "access_token", "auth_token")
}

func containsAny(s string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(s, value) {
			return true
		}
	}
	return false
}
