package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestControlPlaneModelHandlerManagesInferenceEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected discovery path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"dynamic-9b"}]}`))
	}))
	defer server.Close()

	registryPath := filepath.Join(t.TempDir(), "models.json")
	if err := os.WriteFile(registryPath, []byte(`{"deployments":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registryPath)

	db, err := sql.Open("postgres", "host=127.0.0.1 port=1 dbname=none sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	w := NewWall("")
	wallControlPlanes.Store(w, &controlPlane{db: db})
	t.Cleanup(func() { wallControlPlanes.Delete(w) })
	h := controlPlaneModelExperimentHTTPHandler(w, w.Handler())

	draft := map[string]any{
		"id": "lab-gpu", "label": "Lab GPU", "compute": "GPU",
		"endpoint": server.URL + "/v1", "enabled": true, "discover": true,
		"default_for": []string{"farm"}, "max_parallel_workers": 3,
	}
	testResp := requestJSON(t, h, http.MethodPost, "/v1/models/test", draft)
	if testResp.Code == http.StatusMethodNotAllowed {
		t.Fatalf("control-plane test route missing: %d %s", testResp.Code, testResp.Body.String())
	}
	if testResp.Code != http.StatusOK {
		t.Fatalf("test endpoint = %d %s", testResp.Code, testResp.Body.String())
	}

	save := requestJSON(t, h, http.MethodPost, "/v1/models", draft)
	if save.Code == http.StatusMethodNotAllowed {
		t.Fatalf("control-plane save route missing: %d %s", save.Code, save.Body.String())
	}
	if save.Code != http.StatusOK {
		t.Fatalf("save endpoint = %d %s", save.Code, save.Body.String())
	}

	deleted := requestJSON(t, h, http.MethodDelete, "/v1/models/lab-gpu", nil)
	if deleted.Code == http.StatusMethodNotAllowed {
		t.Fatalf("control-plane delete route missing: %d %s", deleted.Code, deleted.Body.String())
	}
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete endpoint = %d %s", deleted.Code, deleted.Body.String())
	}
	registry, err := farm.LoadModelRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Deployment("lab-gpu"); ok {
		t.Fatal("deleted deployment remained in registry")
	}
}
