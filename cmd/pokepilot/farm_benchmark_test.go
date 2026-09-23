package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/benchmark"
	"github.com/maestroi/pokepilot/farm"
)

func TestWriteFarmBenchmarkResult(t *testing.T) {
	dir := t.TempDir()
	primary := &agent.LLMPlanner{BaseURL: "http://localhost:1234/v1", Model: "test-model", MaxTokens: 512}
	router := agent.NewFailoverPlanner(primary, nil)
	stats := &statsPlanner{
		inner:  primary,
		router: router,
		decision: agent.DecisionSettings{
			Backend:       "off",
			MinConfidence: 0.65,
		},
		benchmarkCalls: []agent.LLMCall{{Duration: 250 * time.Millisecond}},
	}
	res := agent.Result{
		Stop:       agent.StopDone,
		StartFrame: 100,
		FinalFrame: 900,
		Initial:    agent.Observation{MapName: "PALLET_TOWN", Controllable: true},
		Final:      agent.Observation{MapName: "PEWTER_GYM", Badges: []string{"Boulder"}, Controllable: true},
		GoalStatus: &agent.GoalStatus{Complete: true, Summary: "badges 1/1", Current: 1, Target: 1},
		Outcomes: []agent.ObjectiveResult{{
			Objective: agent.Objective{Kind: agent.KindGym},
			Outcome:   agent.OutcomeCompleted,
			Final:     agent.Observation{MapName: "PEWTER_GYM", Badges: []string{"Boulder"}, Controllable: true},
		}},
		OutcomeTimings: []agent.ObjectiveTiming{{Frame: 900, Round: 1, WallElapsed: 2 * time.Second}},
		Planning:       agent.PlanningStats{StrategicCalls: 1},
	}
	spec := farm.Spec{
		RunID: "ui-qualification-1", Game: "pokemon-red", Planner: "llm", Goal: farm.GoalFrom("badges:1"),
		LLMProfile: "auto", Seed: 7, FPS: 0, ExperimentID: "qual-group",
		ExperimentArm: "qualification-1-of-3", ExperimentCase: "brock",
	}
	started := time.Unix(1000, 0)
	if err := writeFarmBenchmarkResult(spec, res, stats, started, started.Add(3*time.Second), "", dir, "romsha", 12345); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, farmBenchmarkResultName))
	if err != nil {
		t.Fatal(err)
	}
	var got benchmark.Result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.RunID != spec.RunID || got.Outcome != "completed" || got.Frames != 800 {
		t.Fatalf("result = %+v", got)
	}
	if got.EndCondition != "brock" || got.ExperimentID != "qual-group" || got.ExperimentArm != "qualification-1-of-3" {
		t.Fatalf("qualification identity = %+v", got)
	}
	if got.Configuration.Goal != "badges:1" || got.Configuration.MaxFrames != 12345 {
		t.Fatalf("configuration = %+v", got.Configuration)
	}
	if got.Model.Calls != 1 {
		t.Fatalf("model stats = %+v", got.Model)
	}
}

func TestCollectCheckpointArtifactsIncludesBenchmarkResult(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, farmBenchmarkResultName), []byte(`{"version":1,"run_id":"x","game":"pokemon-red"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	arts, err := collectCheckpointArtifacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 || arts[0].Name != farmBenchmarkResultName || arts[0].MediaType != "application/json" {
		t.Fatalf("artifacts = %+v", arts)
	}
}
