package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloneRunCopiesExecutionSettingsAndRunPolicy(t *testing.T) {
	w := NewWall("")
	h := w.Handler()

	body := `{
		"run_id":"source-run",
		"seed":4242,
		"game":"pokemon-yellow",
		"planner":"llm",
		"starter":"pikachu",
		"dest":"",
		"goal":"Become Champion.",
		"llm_profile":"local",
		"llm_deployment":"xtx-9b",
		"experiment_id":"exp-original",
		"experiment_arm":"a",
		"experiment_case":"case-original",
		"reasoning_effort":"high",
		"fps":120,
		"max_rounds":77,
		"max_frames":900000,
		"endless":true,
		"random_seed":false,
		"play_style":"completionist",
		"purpose":"debug_coverage",
		"risk_tolerance":"cautious",
		"wild_encounters":"fight"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/specs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("enqueue source status = %d body=%s", res.Code, res.Body.String())
	}

	visible := false
	if _, err := w.patchSpectatorRunControl(context.Background(), "source-run", spectatorRunControlPatch{Visible: &visible}); err != nil {
		t.Fatalf("hide source: %v", err)
	}

	cloneReq := httptest.NewRequest(http.MethodPost, "/v1/runs/source-run/clone", nil)
	cloneRes := httptest.NewRecorder()
	h.ServeHTTP(cloneRes, cloneReq)
	if cloneRes.Code != http.StatusCreated {
		t.Fatalf("clone status = %d body=%s", cloneRes.Code, cloneRes.Body.String())
	}
	var result cloneRunResult
	if err := json.NewDecoder(cloneRes.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.RunID == "" || result.RunID == "source-run" {
		t.Fatalf("clone run id = %q", result.RunID)
	}
	if result.ClonedFrom != "source-run" || result.Status != statusQueued {
		t.Fatalf("clone result = %+v", result)
	}

	w.mu.Lock()
	clone := w.tiles[result.RunID]
	queue := append([]string(nil), w.queue...)
	w.mu.Unlock()
	if clone == nil {
		t.Fatal("clone tile missing")
	}
	if clone.Seed != 4242 || clone.Game != "pokemon-yellow" || clone.Planner != "llm" ||
		clone.Starter != "pikachu" || clone.Goal != "Become Champion." ||
		clone.LLMProfile != "local" || clone.LLMDeployment != "xtx-9b" ||
		clone.ReasoningEffort != "high" || clone.FPS != 120 ||
		clone.MaxRounds != 77 || clone.MaxFrames != 900000 ||
		!clone.Endless || clone.RandomSeed {
		t.Fatalf("clone settings differ: %+v", clone)
	}
	if clone.ExperimentID != "" || clone.ExperimentArm != "" || clone.ExperimentCase != "" {
		t.Fatalf("manual clone inherited experiment bookkeeping: %+v", clone)
	}
	if len(queue) != 2 || queue[0] != "source-run" || queue[1] != result.RunID {
		t.Fatalf("queue = %#v", queue)
	}

	if clone.PlayStyle != "completionist" || clone.Purpose != "debug_coverage" || clone.RiskTolerance != "cautious" || clone.WildEncounters != "fight" {
		t.Fatalf("clone run policy = %q/%q/%q/%q", clone.PlayStyle, clone.Purpose, clone.RiskTolerance, clone.WildEncounters)
	}

	control, err := w.spectatorControlSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := control.Runs[result.RunID]; !ok || got.Visible {
		t.Fatalf("clone spectator visibility = %+v, explicit=%v; want hidden", got, ok)
	}
}

func TestCloneRunUnknownSource(t *testing.T) {
	w := NewWall("")
	res := httptest.NewRecorder()
	w.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/runs/missing/clone", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
}
