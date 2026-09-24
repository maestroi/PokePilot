package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func postSpec(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/specs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func TestWallRejectsUnknownDecisionEngine(t *testing.T) {
	h := NewWall("").Handler()
	for _, body := range []string{
		`{"run_id":"bad-backend","planner":"llm","decision_engine":{"backend":"gpt"}}`,
		`{"run_id":"bad-confidence","planner":"llm","decision_engine":{"backend":"jev","min_confidence":1.5}}`,
		`{"run_id":"bad-mode","planner":"llm","decision_engine":{"backend":"jev","mode":"yolo"}}`,
		`{"run_id":"active-battles","planner":"llm","decision_engine":{"backend":"jev","mode":"active","battles":true}}`,
	} {
		if res := postSpec(t, h, body); res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "decision_engine") {
			t.Fatalf("POST %s = %d %s, want 400 naming decision_engine", body, res.Code, res.Body.String())
		}
	}
}

func TestDecisionEngineSelectionFlowsThroughWall(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "wall-state.json")
	ctx := context.Background()

	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())
	if res := postSpec(t, w1.Handler(), `{"run_id":"jev-run","planner":"llm","decision_engine":{"backend":"TypeSafe","mode":"shadow","objectives":true,"failures":true,"battles":true,"min_confidence":0.7}}`); res.Code != http.StatusOK {
		t.Fatalf("enqueue = %d %s", res.Code, res.Body.String())
	}
	enqueueViaHTTP(t, srv1.URL, farm.Spec{RunID: "plain-run", Planner: "llm"})

	want := farm.DecisionEngineSpec{Backend: farm.DecisionBackendJev, Mode: farm.DecisionModeShadow, Objectives: true, Failures: true, Battles: true, MinConfidence: 0.7}
	client := farm.NewClient(srv1.URL)
	leased := map[string]*farm.Spec{}
	for range 2 {
		spec, err := client.Lease(ctx)
		if err != nil || spec == nil {
			t.Fatalf("lease = %v, %v", spec, err)
		}
		leased[spec.RunID] = spec
	}
	if got := leased["jev-run"].DecisionEngine; got == nil || *got != want {
		t.Fatalf("leased jev selection = %+v, want %+v", got, want)
	}
	if got := leased["plain-run"].DecisionEngine; got != nil {
		t.Fatalf("plain run leased a selection %+v; old specs must keep the runner default", got)
	}
	srv1.Close()

	// The selection survives a wall restart and is visible to the operator UI.
	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()
	var restored *farm.DecisionEngineSpec
	for _, run := range getDashboardView(t, srv2.URL).Runs {
		if run.RunID == "jev-run" {
			restored = run.DecisionEngine
		}
	}
	if restored == nil || *restored != want {
		t.Fatalf("restored dashboard selection = %+v, want %+v", restored, want)
	}

	// A clone reproduces the experiment settings as an independent copy.
	cloneRes := httptest.NewRecorder()
	w2.Handler().ServeHTTP(cloneRes, httptest.NewRequest(http.MethodPost, "/v1/runs/jev-run/clone", nil))
	if cloneRes.Code != http.StatusCreated {
		t.Fatalf("clone = %d %s", cloneRes.Code, cloneRes.Body.String())
	}
	w2.mu.Lock()
	defer w2.mu.Unlock()
	var clone *Tile
	for id, tile := range w2.tiles {
		if id != "jev-run" && id != "plain-run" {
			clone = tile
		}
	}
	if clone == nil || clone.DecisionEngine == nil || *clone.DecisionEngine != want {
		t.Fatalf("clone selection = %+v", clone)
	}
	if clone.DecisionEngine == w2.tiles["jev-run"].DecisionEngine {
		t.Fatal("clone shares the source's selection pointer")
	}
}

func TestDecisionEngineDeploymentResolvesFromRegistry(t *testing.T) {
	registry := writeModelRegistry(t, []farm.ModelDeployment{
		{ID: "qwen-9b", Label: "Qwen 9B", ModelID: "qwen3.5-9b", Compute: "RTX 4090", Endpoint: "http://4090/v1", APIModel: "qwen3.5-9b", Enabled: true},
		{ID: "typesafe-jev", Label: "TypeSafe Jev", ModelID: "jev-latest", Compute: "typesafe-cloud", Endpoint: "https://api.typesafe.ai/v1", APIModel: "jev-latest", Protocol: "typesafe-choice", TokenEnv: "TYPESAFE_API_KEY", Enabled: true},
	})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	w := NewWall(t.TempDir())
	h := modelExperimentHTTPHandler(w, w.Handler())

	// The client names only the deployment; a forged identity is discarded.
	res := postSpec(t, h, `{"run_id":"jev-shadow","planner":"llm","llm_deployment":"qwen-9b","decision_engine":{"deployment":"typesafe-jev","mode":"shadow","battles":true,"inference":{"endpoint":"https://evil.example/v1","token_env":"AWS_SECRET_ACCESS_KEY"}}}`)
	if res.Code != http.StatusOK {
		t.Fatalf("enqueue = %d %s", res.Code, res.Body.String())
	}
	w.mu.Lock()
	got := w.tiles["jev-shadow"].DecisionEngine.Clone()
	w.mu.Unlock()
	if got == nil || got.Backend != farm.DecisionBackendJev || got.Deployment != "typesafe-jev" || got.Mode != farm.DecisionModeShadow || !got.Battles {
		t.Fatalf("stored selection = %+v", got)
	}
	if id := got.Inference; id == nil || id.Endpoint != "https://api.typesafe.ai/v1" || id.APIModel != "jev-latest" || id.TokenEnv != "TYPESAFE_API_KEY" || id.Protocol != farm.ProtocolTypeSafeChoice {
		t.Fatalf("stored identity = %+v", got.Inference)
	}

	for body, want := range map[string]string{
		// A choice-only API cannot be the strategist.
		`{"run_id":"jev-strategist","planner":"llm","llm_deployment":"typesafe-jev"}`:                           "decision engine",
		`{"run_id":"unknown-decision","planner":"llm","decision_engine":{"deployment":"nope","mode":"shadow"}}`: "decision_engine.deployment",
	} {
		if res := postSpec(t, h, body); res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), want) {
			t.Errorf("POST %s = %d %s, want 400 naming %q", body, res.Code, res.Body.String(), want)
		}
	}

	// An explicit off ignores the deployment instead of binding it.
	if res := postSpec(t, h, `{"run_id":"off-run","planner":"llm","decision_engine":{"deployment":"typesafe-jev","mode":"off"}}`); res.Code != http.StatusOK {
		t.Fatalf("off enqueue = %d %s", res.Code, res.Body.String())
	}
	w.mu.Lock()
	off := w.tiles["off-run"].DecisionEngine
	w.mu.Unlock()
	if off == nil || off.Enabled() || off.Inference != nil {
		t.Fatalf("off selection = %+v", off)
	}
}

func TestCatalogKeepsDecisionSummaryButNotTheLiveFeed(t *testing.T) {
	summary := &farm.DecisionSummary{}
	summary.Observe(farm.TypedDecisionRecord{Kind: "objective_selection", Confidence: 0.9})
	stats := &farm.LLMStats{
		DecisionRecords: []farm.TypedDecisionRecord{{Kind: "objective_selection", Choice: "1"}},
		DecisionSummary: summary,
	}
	row := sanitizeCatalogRow(tileRow{RunID: "r", Stats: stats})
	if len(row.Stats.DecisionRecords) != 0 || row.Stats.DecisionSummary == nil || row.Stats.DecisionSummary.Kinds["objective_selection"].Calls != 1 {
		t.Fatalf("catalog stats = %+v", row.Stats)
	}
	if len(stats.DecisionRecords) != 1 {
		t.Fatal("sanitizing the catalog row cleared the live tile's feed")
	}
}
