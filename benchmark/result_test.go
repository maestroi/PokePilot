package benchmark

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/agent"
)

func TestBuildCapturesSplitsTimingCountersAndModelCost(t *testing.T) {
	start := time.Unix(100, 0)
	res := agent.Result{
		Stop:       agent.StopDone,
		StartFrame: 100,
		FinalFrame: 300,
		Initial:    agent.Observation{MapName: "PALLET_TOWN", Controllable: true},
		Final:      agent.Observation{MapName: "CERULEAN_CITY", Badges: []string{"Boulder", "Cascade"}},
		GoalStatus: &agent.GoalStatus{Complete: true},
		Completed:  []agent.Objective{{Kind: agent.KindGoTo}, {Kind: agent.KindGym}},
		Planning:   agent.PlanningStats{StrategicCalls: 2, FastCalls: 1, ReplanReasons: map[string]int{"initial": 1, "objective_failed": 1}},
	}
	first := agent.ObjectiveResult{
		Objective: agent.Objective{Kind: agent.KindGoTo, Place: "pewter city"},
		Outcome:   agent.OutcomeCompleted,
		Final:     agent.Observation{MapName: "PEWTER_CITY", Badges: []string{"Boulder"}},
		Travel:    &agent.TravelEvidence{Battles: 2, Flees: 1, Replans: 1},
	}
	second := agent.ObjectiveResult{
		Objective: agent.Objective{Kind: agent.KindGym},
		Outcome:   agent.OutcomeCompleted,
		Final:     res.Final,
	}
	res.Outcomes = []agent.ObjectiveResult{first, second}
	res.OutcomeTimings = []agent.ObjectiveTiming{
		{Frame: 160, Round: 1, WallElapsed: 2 * time.Second},
		{Frame: 300, Round: 2, WallElapsed: 5 * time.Second},
	}
	profile := Profile{Game: "pokemon-red", Milestones: []MilestoneDefinition{
		{ID: "brock", Name: "Brock", Match: func(obs agent.Observation) bool { return len(obs.Badges) >= 1 }},
		{ID: "misty", Name: "Misty", Match: func(obs agent.Observation) bool { return len(obs.Badges) >= 2 }},
	}}
	result := Build(BuildInput{
		RunID: "run-test", Commit: "abc", Game: "pokemon-red", ROMSHA256: "deadbeef", Mode: "speedrun",
		EndCondition: "misty", Profile: profile, AgentResult: res,
		Calls: []agent.LLMCall{
			{Duration: time.Second, Strategic: true},
			{Duration: 500 * time.Millisecond},
		},
		StartedAt: start, FinishedAt: start.Add(6 * time.Second),
	})
	if result.Outcome != "completed" || result.Frames != 200 {
		t.Fatalf("result = outcome %q frames %d", result.Outcome, result.Frames)
	}
	if len(result.Milestones) != 3 {
		t.Fatalf("milestones = %+v", result.Milestones)
	}
	if got := result.Milestones[1].FramesSincePrevious; got != 60 {
		t.Fatalf("Brock delta frames = %d, want 60", got)
	}
	if got := result.Milestones[2].FramesSincePrevious; got != 140 {
		t.Fatalf("Misty delta frames = %d, want 140", got)
	}
	if got := result.Milestones[2].WallSincePrevious; got != 3 {
		t.Fatalf("Misty wall delta = %.2f, want 3", got)
	}
	if result.Timing["navigation"].Frames != 60 || result.Timing["battle"].Frames != 140 {
		t.Fatalf("timing = %+v", result.Timing)
	}
	if result.Timing["strategist_inference"].WallSeconds != 1 || result.Timing["fast_inference"].WallSeconds != .5 {
		t.Fatalf("inference timing = %+v", result.Timing)
	}
	if result.Counters["battles"] != 3 || result.Counters["wild_battles"] != 2 || result.Counters["trainer_battles"] != 1 {
		t.Fatalf("counters = %+v", result.Counters)
	}
	if result.Counters["replans"] != 2 || result.Model.Calls != 2 || result.Model.StrategistCalls != 1 {
		t.Fatalf("planner/model telemetry = counters %+v model %+v", result.Counters, result.Model)
	}
}

func TestCheckpointSourceDoesNotPretendEarlierMilestonesWereCrossed(t *testing.T) {
	obs := agent.Observation{Badges: []string{"Boulder", "Cascade", "Thunder", "Rainbow", "Soul", "Marsh"}}
	res := agent.Result{
		StartFrame: 500, FinalFrame: 700, Initial: obs,
		Outcomes:       []agent.ObjectiveResult{{Objective: agent.Objective{Kind: agent.KindGym}, Outcome: agent.OutcomeCompleted, Final: agent.Observation{Badges: append(append([]string(nil), obs.Badges...), "Volcano")}}},
		OutcomeTimings: []agent.ObjectiveTiming{{Frame: 700, Round: 1, WallElapsed: time.Second}},
	}
	profile := Profile{Milestones: []MilestoneDefinition{
		{ID: "sabrina", Match: func(o agent.Observation) bool { return len(o.Badges) >= 6 }},
		{ID: "blaine", Match: func(o agent.Observation) bool { return len(o.Badges) >= 7 }},
	}}
	got := splits(profile, Source{Kind: "checkpoint"}, res)
	if len(got) != 2 || got[0].ID != "checkpoint_start" || got[1].ID != "blaine" {
		t.Fatalf("checkpoint splits = %+v", got)
	}
}

func TestBuildFailureUsesFarmFingerprintAndHistory(t *testing.T) {
	now := time.Unix(200, 0)
	res := agent.Result{
		Stop: agent.StopFailed, StartFrame: 10, FinalFrame: 40,
		Initial:  agent.Observation{Location: "route 1", Controllable: true},
		Final:    agent.Observation{Location: "route 1", MapName: "ROUTE_1", X: 3, Y: 4, Controllable: true},
		Err:      errors.New("outer: controller blocked"),
		Planning: agent.PlanningStats{ReplanReasons: map[string]int{"objective_failed": 2}},
	}
	res.Outcomes = []agent.ObjectiveResult{{
		Objective: agent.Objective{Kind: agent.KindGoTo, Place: "viridian city"},
		Outcome:   agent.OutcomeBlocked, Cause: agent.FailureCauseID("route_blocked"),
		Summary: "blocked at route 1", Final: res.Final, Terminal: true,
	}}
	res.OutcomeTimings = []agent.ObjectiveTiming{{Frame: 40, Round: 3, WallElapsed: 2 * time.Second}}
	result := Build(BuildInput{
		RunID: "failed", Commit: "abc", Game: "pokemon-red", ROMSHA256: "hash", Mode: "speedrun",
		EndCondition: "brock", AgentResult: res, StartedAt: now, FinishedAt: now.Add(2 * time.Second),
	})
	if len(result.Failures) != 1 {
		t.Fatalf("failures = %+v", result.Failures)
	}
	failure := result.Failures[0]
	if failure.Fingerprint == "" || !strings.HasPrefix(failure.Fingerprint, "sha256:") {
		t.Fatalf("fingerprint = %q", failure.Fingerprint)
	}
	if failure.Round != 3 || failure.Objective == "" || len(failure.Recent) != 1 {
		t.Fatalf("failure detail = %+v", failure)
	}
}

func TestResultSerializationAndVersionHandling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "benchmark-result.json")
	want := Result{Version: ResultVersion, RunID: "r1", Game: "pokemon-red", Outcome: "completed"}
	if err := WriteJSON(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadResults(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RunID != "r1" {
		t.Fatalf("loaded = %+v", got)
	}
	if err := os.WriteFile(path, []byte(`{"version":99,"run_id":"bad","game":"pokemon-red"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResults(path); err == nil {
		t.Fatal("future schema version unexpectedly accepted")
	}
}

func TestMaterializeCheckpointsCopiesStateAndAgentMemory(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "ring")
	outputDir := filepath.Join(dir, "run")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(sourceDir, "round-002-frame-0000000100-next.state")
	if err := os.WriteFile(state, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(strings.TrimSuffix(state, ".state")+".knowledge-v1.json", []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Result{Milestones: []Split{{ID: "brock", Round: 1}}}
	if err := MaterializeCheckpoints(&result, sourceDir, outputDir, ""); err != nil {
		t.Fatal(err)
	}
	if result.Milestones[0].Checkpoint != "checkpoints/brock.state" {
		t.Fatalf("checkpoint = %q", result.Milestones[0].Checkpoint)
	}
	for _, name := range []string{"brock.state", "brock.knowledge-v1.json"} {
		if _, err := os.Stat(filepath.Join(outputDir, "checkpoints", name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
}

func TestSummarizeDoesNotAverageFailuresIntoCompletionTime(t *testing.T) {
	results := []Result{
		{Version: 1, RunID: "a", Game: "pokemon-red", Outcome: "completed", Frames: 100, WallSeconds: 10, Counters: map[string]int64{"replans": 2}},
		{Version: 1, RunID: "b", Game: "pokemon-red", Outcome: "completed", Frames: 200, WallSeconds: 20, Counters: map[string]int64{"replans": 4}},
		{Version: 1, RunID: "c", Game: "pokemon-red", Outcome: "failed", Frames: 10000, WallSeconds: 1000, Failures: []Failure{{Fingerprint: "sha256:x"}}, Counters: map[string]int64{"replans": 9}},
	}
	agg := Summarize(results)
	if agg.Completed != 2 || agg.Runs != 3 || agg.MedianFrames != 150 || agg.MedianWallSeconds != 15 {
		t.Fatalf("aggregate = %+v", agg)
	}
	if agg.FailureFingerprints["sha256:x"] != 1 {
		t.Fatalf("failures = %+v", agg.FailureFingerprints)
	}
}

func TestCompareTextMakesReliabilityRegressionObvious(t *testing.T) {
	base := Aggregate{Runs: 5, Completed: 5, CompletionRate: 1, MedianFrames: 100}
	candidate := Aggregate{Runs: 5, Completed: 4, CompletionRate: .8, MedianFrames: 80}
	text := CompareText(base, candidate)
	if !strings.Contains(text, "RELIABILITY REGRESSION") || !strings.Contains(text, "Descriptive comparison only") {
		t.Fatalf("comparison:\n%s", text)
	}
}

func TestSanitizeSettingsAndEndpointExcludeSecrets(t *testing.T) {
	got := SanitizeSettings(map[string]string{
		"feature":   "on",
		"api_token": "secret",
		"endpoint":  "https://user:pass@example.test/v1?api_key=hidden",
	})
	if got["feature"] != "on" {
		t.Fatalf("feature lost: %+v", got)
	}
	if _, ok := got["api_token"]; ok {
		t.Fatalf("secret persisted: %+v", got)
	}
	if strings.Contains(got["endpoint"], "user") || strings.Contains(got["endpoint"], "hidden") {
		t.Fatalf("endpoint leaked credentials: %q", got["endpoint"])
	}
}
