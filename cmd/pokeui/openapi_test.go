package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnnotateOperatorArtifactDownloads(t *testing.T) {
	payload := map[string]any{
		"run_id": "run-abc",
		"artifacts": []any{
			map[string]any{"name": "round-1.state", "inline": true},
			map[string]any{"name": "path/weird.state"},
		},
	}
	annotateOperatorArtifactDownloads("https://pokemon.labstack.cc", "run-abc", payload)

	download, _ := payload["download"].(map[string]any)
	if download["method"] != http.MethodGet || download["authorization"] != "none" {
		t.Fatalf("download = %#v", download)
	}
	if payload["http_base"] != "https://pokemon.labstack.cc" {
		t.Fatalf("http_base = %v", payload["http_base"])
	}
	if download["openapi"] != "https://pokemon.labstack.cc/openapi.json" {
		t.Fatalf("openapi = %v", download["openapi"])
	}
	note, _ := download["note"].(string)
	if note == "" || !containsAll(note, "/mcp", "Authorization") {
		t.Fatalf("note = %q", note)
	}

	arts := payload["artifacts"].([]any)
	first := arts[0].(map[string]any)
	if first["content_url"] != "https://pokemon.labstack.cc/v1/runs/run-abc/artifacts/round-1.state/content" {
		t.Fatalf("content_url = %v", first["content_url"])
	}
	second := arts[1].(map[string]any)
	if second["content_url"] != "https://pokemon.labstack.cc/v1/runs/run-abc/artifacts/path%2Fweird.state/content" {
		t.Fatalf("encoded content_url = %v", second["content_url"])
	}
}

func TestEnsureRunPrefix(t *testing.T) {
	if got := ensureRunPrefix("17ub24cwckopa1ts97iyldmzng"); got != "run-17ub24cwckopa1ts97iyldmzng" {
		t.Fatalf("got %q", got)
	}
	if got := ensureRunPrefix("run-abc"); got != "run-abc" {
		t.Fatalf("already prefixed: %q", got)
	}
}

func TestMCPGetRunArtifactsIncludesContentURL(t *testing.T) {
	t.Setenv("POKEPILOT_RUN_BASE_URL", "https://pokemon.labstack.cc")
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/run-abc/artifacts" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"run_id": "run-abc", "attempt": 1,
			"artifacts": []map[string]any{{"name": "fail.state", "inline": true}},
		})
	}))
	t.Cleanup(wall.Close)

	control := &mcpControl{wallBase: wall.URL, publicBase: "https://pokemon.labstack.cc", http: wall.Client()}
	_, out, err := control.getRunArtifacts(context.Background(), nil, mcpRunInput{RunID: "run-abc"})
	if err != nil {
		t.Fatal(err)
	}
	arts, _ := out["artifacts"].([]any)
	if len(arts) != 1 {
		t.Fatalf("artifacts = %#v", out["artifacts"])
	}
	got := arts[0].(map[string]any)["content_url"]
	want := "https://pokemon.labstack.cc/v1/runs/run-abc/artifacts/fail.state/content"
	if got != want {
		t.Fatalf("content_url = %v, want %s", got, want)
	}
}

func TestMCPGetRunArtifactsRetriesRunPrefix(t *testing.T) {
	var paths []string
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/v1/runs/run-abc/artifacts" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "run not found"}) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"run_id": "run-abc", "artifacts": []map[string]any{{"name": "fail.state"}},
		})
	}))
	t.Cleanup(wall.Close)

	control := &mcpControl{wallBase: wall.URL, publicBase: "https://example.test", http: wall.Client()}
	_, out, err := control.getRunArtifacts(context.Background(), nil, mcpRunInput{RunID: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/v1/runs/abc/artifacts" || paths[1] != "/v1/runs/run-abc/artifacts" {
		t.Fatalf("paths = %v", paths)
	}
	if out["run_id"] != "run-abc" {
		t.Fatalf("run_id = %v", out["run_id"])
	}
}

func TestMCPGetRunDebugIncludesContentURL(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/run-abc/debug" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"run":       map[string]any{"run_id": "run-abc", "status": "done"},
			"artifacts": []map[string]any{{"name": "run.gbrun"}},
		})
	}))
	t.Cleanup(wall.Close)

	control := &mcpControl{wallBase: wall.URL, publicBase: "https://pokemon.labstack.cc", http: wall.Client()}
	_, out, err := control.getRunDebug(context.Background(), nil, mcpRunInput{RunID: "run-abc"})
	if err != nil {
		t.Fatal(err)
	}
	arts, _ := out["artifacts"].([]any)
	got := arts[0].(map[string]any)["content_url"]
	want := "https://pokemon.labstack.cc/v1/runs/run-abc/artifacts/run.gbrun/content"
	if got != want {
		t.Fatalf("debug content_url = %v, want %s", got, want)
	}
}

func TestOperatorServesOpenAPI(t *testing.T) {
	ui := httptest.NewServer(handler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var spec map[string]any
	if err := json.NewDecoder(res.Body).Decode(&spec); err != nil {
		t.Fatal(err)
	}
	if spec["openapi"] == nil {
		t.Fatalf("missing openapi version: %#v", spec)
	}
	paths, _ := spec["paths"].(map[string]any)
	if paths["/v1/runs/{id}/artifacts/{name}/content"] == nil {
		t.Fatalf("missing content path: %#v", paths)
	}
	info, _ := spec["info"].(map[string]any)
	desc, _ := info["description"].(string)
	if !containsAll(desc, "/mcp", "Authorization") {
		t.Fatalf("info.description = %q", desc)
	}
}

func TestSpectatorHidesOpenAPI(t *testing.T) {
	ui := httptest.NewServer(spectatorHandler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("spectator openapi = %d, want 404", res.StatusCode)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
