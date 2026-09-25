package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestExperimentAnalyticsUsePersistedRunHistory(t *testing.T) {
	registry := writeModelRegistry(t, []farm.ModelDeployment{
		{ID: "model-a", ModelID: "a", Compute: "gpu-a", Endpoint: "http://a/v1", APIModel: "a", Enabled: true},
		{ID: "model-b", ModelID: "b", Compute: "gpu-b", Endpoint: "http://b/v1", APIModel: "b", Enabled: true},
	})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	t.Setenv("POKEPILOT_ROM_SHA256", "rom-sha")
	t.Setenv("POKEPILOT_PROMPT_SHA256", "prompt-sha")

	w := NewWall("")
	w.Version = "git-sha"
	if err := w.SetCatalogPath(filepath.Join(t.TempDir(), "runs.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.CloseCatalog() })
	h := modelExperimentHTTPHandler(w, w.Handler())

	created := requestJSON(t, h, http.MethodPost, "/v1/experiments", farm.ExperimentRequest{
		ArmA:  farm.ExperimentArm{Deployment: "model-a"},
		ArmB:  farm.ExperimentArm{Deployment: "model-b"},
		Seeds: []int64{1},
		Game:  "pokemon-red",
		Goal:  "Earn the Boulder Badge.",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var experiment struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &experiment); err != nil {
		t.Fatal(err)
	}

	for _, arm := range []string{"a", "b"} {
		runID := experiment.ID + "-seed-1-" + arm
		w.mu.Lock()
		tile := w.tiles[runID]
		if tile == nil {
			w.mu.Unlock()
			t.Fatalf("missing experiment tile %s", runID)
		}
		tile.Status = statusDone
		tile.Finished = true
		tile.Reason = "complete"
		tile.Frame = 12345
		tile.EndedAt = time.Now()
		tile.Player = &farm.Player{Badges: []string{"Boulder"}}
		tile.Stats = &farm.LLMStats{Rounds: 7, Calls: 3, StrategicCalls: 2}
		w.mu.Unlock()
	}
	if err := w.syncCatalogFromRAM(true); err != nil {
		t.Fatal(err)
	}
	w.evictCatalogFinished()

	w.mu.Lock()
	remaining := len(w.tiles)
	w.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("finished experiment tiles still in RAM: %d", remaining)
	}

	res := requestJSON(t, h, http.MethodGet, "/v1/experiments/"+experiment.ID, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("experiment = %d %s", res.Code, res.Body.String())
	}
	var view struct {
		ArmA armAggregate `json:"arm_a"`
		ArmB armAggregate `json:"arm_b"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]armAggregate{"a": view.ArmA, "b": view.ArmB} {
		if got.Runs != 1 || got.Done != 1 || got.GoalSuccesses != 1 || got.BoulderSuccesses != 1 || got.Rounds != 7 || got.Frames != 12345 {
			t.Fatalf("arm %s aggregate = %#v", name, got)
		}
	}
}
