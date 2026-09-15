package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func writeModelRegistry(t *testing.T, deployments []farm.ModelDeployment) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "models.json")
	data, err := json.Marshal(farm.ModelRegistry{Deployments: deployments})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func requestJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func TestExperimentGeneratesMatchedDeploymentRuns(t *testing.T) {
	registry := writeModelRegistry(t, []farm.ModelDeployment{
		{ID: "qwen-27b-7900", Label: "Qwen 27B / 7900", ModelID: "qwen3.8-27b", Revision: "sha256:27", Compute: "7900 XTX", Endpoint: "http://7900/v1", APIModel: "pokepilot-7900", Enabled: true, LegacyProfile: "auto"},
		{ID: "qwen-4b-4090", Label: "Qwen 4B / 4090", ModelID: "qwen3.5-4b", Revision: "sha256:4", Compute: "RTX 4090", Endpoint: "http://4090/v1", APIModel: "pokepilot-4090", Enabled: true, LegacyProfile: "gpu"},
	})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	t.Setenv("POKEPILOT_ROM_SHA256", "rom-sha")
	t.Setenv("POKEPILOT_PROMPT_SHA256", "prompt-sha")
	w := NewWall(t.TempDir())
	w.Version = "git-sha"
	h := modelExperimentHTTPHandler(w, w.Handler())

	create := requestJSON(t, h, http.MethodPost, "/v1/experiments", farm.ExperimentRequest{
		Name: "brock-27b-v-4b", Goal: "Earn the Boulder Badge.", Starter: "squirtle", Seeds: []int64{101, 202},
		ArmA: farm.ExperimentArm{Name: "27B", Deployment: "qwen-27b-7900"}, ArmB: farm.ExperimentArm{Name: "4B", Deployment: "qwen-4b-4090"},
		PlayStyle: "speedrunner", RiskTolerance: "balanced", WildEncounters: "flee", ReasoningEffort: "medium", FPS: 0, MaxRounds: 30, MaxFrames: 500000,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", create.Code, create.Body.String())
	}
	var created struct {
		ID         string       `json:"id"`
		TotalPairs int          `json:"total_pairs"`
		Pairs      []pairResult `json:"pairs"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.TotalPairs != 2 {
		t.Fatalf("created = %#v", created)
	}
	if len(created.Pairs) != 2 || !created.Pairs[0].Comparable || !created.Pairs[1].Comparable {
		t.Fatalf("pairs = %#v", created.Pairs)
	}

	w.mu.Lock()
	queued := append([]string(nil), w.queue...)
	w.mu.Unlock()
	if len(queued) != 4 {
		t.Fatalf("queued = %d, want 4", len(queued))
	}

	lease := requestJSON(t, h, http.MethodPost, "/v1/lease", map[string]any{})
	if lease.Code != http.StatusOK {
		t.Fatalf("lease = %d %s", lease.Code, lease.Body.String())
	}
	var spec farm.Spec
	if err := json.Unmarshal(lease.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	if spec.LLMDeployment == "" || spec.Inference == nil || spec.Inference.DeploymentID != spec.LLMDeployment {
		t.Fatalf("leased spec missing inference identity: %#v", spec)
	}
	if spec.ExperimentID != created.ID || spec.ExperimentCase == "" || (spec.ExperimentArm != "a" && spec.ExperimentArm != "b") {
		t.Fatalf("experiment identity = %#v", spec)
	}

	dashboard := requestJSON(t, h, http.MethodGet, "/v1/dashboard", nil)
	if dashboard.Code != http.StatusOK {
		t.Fatalf("dashboard = %d", dashboard.Code)
	}
	var doc struct {
		Runs []map[string]any `json:"runs"`
	}
	if err := json.Unmarshal(dashboard.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, run := range doc.Runs {
		if run["run_id"] == spec.RunID {
			found = run["llm_deployment"] == spec.LLMDeployment && run["inference"] != nil
		}
	}
	if !found {
		t.Fatalf("dashboard did not expose deployment identity: %s", dashboard.Body.String())
	}
}

func TestBusyModelHostLeavesRunQueued(t *testing.T) {
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/status":
			_ = json.NewEncoder(w).Encode(modelHostStatus{State: "ready", DeploymentID: "other-model", ActiveLeases: 1, Health: "ready"})
		default:
			t.Fatalf("unexpected model-host call %s", r.URL.Path)
		}
	}))
	defer host.Close()
	registry := writeModelRegistry(t, []farm.ModelDeployment{{ID: "qwen-4b", Label: "4B", ModelID: "qwen-4b", Compute: "4090", Endpoint: "http://4090/v1", APIModel: "pokepilot-4090", Enabled: true, ControlURL: host.URL, LegacyProfile: "gpu"}})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	w := NewWall("")
	h := modelExperimentHTTPHandler(w, w.Handler())

	enqueue := requestJSON(t, h, http.MethodPost, "/v1/specs", map[string]any{"run_id": "busy-run", "seed": 7, "planner": "llm", "goal": "Earn the Boulder Badge.", "llm_deployment": "qwen-4b"})
	if enqueue.Code < 200 || enqueue.Code >= 300 {
		t.Fatalf("enqueue = %d %s", enqueue.Code, enqueue.Body.String())
	}
	lease := requestJSON(t, h, http.MethodPost, "/v1/lease", map[string]any{})
	if lease.Code != http.StatusNoContent {
		t.Fatalf("lease = %d %s", lease.Code, lease.Body.String())
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.queue) != 1 || w.queue[0] != "busy-run" {
		t.Fatalf("queue = %#v", w.queue)
	}
}

func TestExperimentMetadataPersistsAcrossControllerRestart(t *testing.T) {
	registry := writeModelRegistry(t, []farm.ModelDeployment{{ID: "a", Label: "A", ModelID: "a", Compute: "7900", Endpoint: "http://a/v1", APIModel: "a", Enabled: true}, {ID: "b", Label: "B", ModelID: "b", Compute: "4090", Endpoint: "http://b/v1", APIModel: "b", Enabled: true}})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	dumps := t.TempDir()
	w := NewWall(dumps)
	h := modelExperimentHTTPHandler(w, w.Handler())
	created := requestJSON(t, h, http.MethodPost, "/v1/experiments", farm.ExperimentRequest{ArmA: farm.ExperimentArm{Deployment: "a"}, ArmB: farm.ExperimentArm{Deployment: "b"}, Seeds: []int64{1}, Goal: "Earn the Boulder Badge."})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}

	restarted := modelExperimentHTTPHandler(w, w.Handler())
	listed := requestJSON(t, restarted, http.MethodGet, "/v1/experiments", nil)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte("total_pairs")) {
		t.Fatalf("list after restart = %d %s", listed.Code, listed.Body.String())
	}
}
