package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestExperimentExcludesPairsMissingRequiredComparabilityIdentity(t *testing.T) {
	registry := writeModelRegistry(t, []farm.ModelDeployment{
		{ID: "a", ModelID: "model-a", Compute: "gpu-a", Endpoint: "http://a/v1", APIModel: "a", Enabled: true},
		{ID: "b", ModelID: "model-b", Compute: "gpu-b", Endpoint: "http://b/v1", APIModel: "b", Enabled: true},
	})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	// Deliberately leave ROM/prompt identities empty. Matching empty strings must
	// never make a benchmark look comparable.
	w := NewWall("")
	w.Version = "git-sha"
	h := modelExperimentHTTPHandler(w, w.Handler())

	created := requestJSON(t, h, http.MethodPost, "/v1/experiments", farm.ExperimentRequest{
		Game: "pokemon-red", Goal: "Earn the Boulder Badge.", Seeds: []int64{7},
		ArmA: farm.ExperimentArm{Deployment: "a", MaxParallelWorkers: 1},
		ArmB: farm.ExperimentArm{Deployment: "b", MaxParallelWorkers: 1},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var createdView struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdView); err != nil {
		t.Fatal(err)
	}

	for _, arm := range []string{"a", "b"} {
		id := createdView.ID + "-seed-7-" + arm
		w.mu.Lock()
		tile := w.tiles[id]
		tile.Status = statusDone
		tile.Finished = true
		tile.Reason = "done"
		tile.Frame = 100
		tile.EndedAt = time.Now()
		tile.Player = &farm.Player{Badges: []string{"Boulder"}}
		tile.Stats = &farm.LLMStats{Rounds: 3, GoalComplete: true}
		w.mu.Unlock()
	}

	res := requestJSON(t, h, http.MethodGet, "/v1/experiments/"+createdView.ID, nil)
	var view struct {
		ArmA   armAggregate            `json:"arm_a"`
		ArmB   armAggregate            `json:"arm_b"`
		Paired experimentPairedSummary `json:"paired"`
		Pairs  []pairResult            `json:"pairs"`
	}
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &view) != nil {
		t.Fatalf("view = %d %s", res.Code, res.Body.String())
	}
	if view.Paired.ExcludedPairs != 1 || view.Paired.ComparablePairs != 0 {
		t.Fatalf("paired = %#v", view.Paired)
	}
	if view.ArmA.Runs != 0 || view.ArmB.Runs != 0 {
		t.Fatalf("non-comparable runs contaminated aggregates: a=%#v b=%#v", view.ArmA, view.ArmB)
	}
	if len(view.Pairs) != 1 || view.Pairs[0].Comparable || !strings.Contains(view.Pairs[0].Reason, "ROM identity") || !strings.Contains(view.Pairs[0].Reason, "prompt identity") {
		t.Fatalf("pair = %#v", view.Pairs)
	}
}

func TestExperimentAggregatesGenericGoalAndIdentity(t *testing.T) {
	registry := writeModelRegistry(t, []farm.ModelDeployment{
		{ID: "a", ModelID: "model-a", Revision: "rev-a", Quantization: "q4", Compute: "gpu-a", Endpoint: "http://a/v1", APIModel: "a", Enabled: true},
		{ID: "b", ModelID: "model-b", Revision: "rev-b", Quantization: "q8", Compute: "gpu-b", Endpoint: "http://b/v1", APIModel: "b", Enabled: true},
	})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	t.Setenv("POKEPILOT_ROM_SHA256", "rom-sha")
	t.Setenv("POKEPILOT_PROMPT_SHA256", "prompt-sha")
	w := NewWall("")
	w.Version = "git-sha"
	h := modelExperimentHTTPHandler(w, w.Handler())

	created := requestJSON(t, h, http.MethodPost, "/v1/experiments", farm.ExperimentRequest{
		Game: "pokemon-blue", Goal: "Reach Cerulean City.", Seeds: []int64{11},
		ArmA: farm.ExperimentArm{Deployment: "a", MaxParallelWorkers: 1},
		ArmB: farm.ExperimentArm{Deployment: "b", MaxParallelWorkers: 1},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var createdView struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdView); err != nil {
		t.Fatal(err)
	}

	for i, arm := range []string{"a", "b"} {
		id := createdView.ID + "-seed-11-" + arm
		w.mu.Lock()
		tile := w.tiles[id]
		tile.Status = statusDone
		tile.Finished = true
		tile.Reason = "done"
		tile.Frame = uint64(1000 + i*200)
		tile.EndedAt = tile.QueuedAt.Add(time.Duration(20+i*5) * time.Second)
		tile.Stats = &farm.LLMStats{
			Rounds: 5 + i, GoalComplete: true, StrategicCalls: 2,
			StrategicSeconds: 4, PlanExecutions: 3, StepsSkipped: 1,
			StrategicRecords: []farm.StrategicCallRecord{{DurationSeconds: 2, PlanSteps: []string{"one", "two"}}},
		}
		w.mu.Unlock()
	}

	res := requestJSON(t, h, http.MethodGet, "/v1/experiments/"+createdView.ID, nil)
	var view struct {
		ArmA     armAggregate            `json:"arm_a"`
		ArmB     armAggregate            `json:"arm_b"`
		Paired   experimentPairedSummary `json:"paired"`
		Identity experimentIdentityView  `json:"identity"`
	}
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &view) != nil {
		t.Fatalf("view = %d %s", res.Code, res.Body.String())
	}
	if view.Paired.ComparablePairs != 1 || view.Paired.CompletedPairs != 1 || view.Paired.Ties != 1 || view.Paired.ExcludedPairs != 0 {
		t.Fatalf("paired = %#v", view.Paired)
	}
	if view.ArmA.GoalSuccesses != 1 || view.ArmA.BoulderSuccesses != 0 || view.ArmA.MedianRoundsToGoal != 5 || view.ArmA.MedianFramesToGoal != 1000 || view.ArmA.AvgRunSeconds != 20 {
		t.Fatalf("arm a = %#v", view.ArmA)
	}
	if view.ArmB.GoalSuccesses != 1 || view.ArmB.MedianRoundsToGoal != 6 || view.ArmB.MedianFramesToGoal != 1200 || view.ArmB.AvgRunSeconds != 25 {
		t.Fatalf("arm b = %#v", view.ArmB)
	}
	if view.Identity.Game != "pokemon-blue" || view.Identity.GitRevision != "git-sha" || view.Identity.ROMIdentity != "rom-sha" || view.Identity.PromptIdentity != "prompt-sha" {
		t.Fatalf("identity = %#v", view.Identity)
	}
	if view.Identity.ArmA.ModelID != "model-a" || view.Identity.ArmB.ModelID != "model-b" || view.Identity.ArmA.Revision != "rev-a" || view.Identity.ArmB.Revision != "rev-b" {
		t.Fatalf("model identities = a:%#v b:%#v", view.Identity.ArmA, view.Identity.ArmB)
	}
}
