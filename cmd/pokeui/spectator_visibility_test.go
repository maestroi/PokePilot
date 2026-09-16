package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSpectatorVisibilityFiltersFeedAndMarksFeatured(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/spectator/control" {
			http.NotFound(res, req)
			return
		}
		json.NewEncoder(res).Encode(wallSpectatorControl{
			FeaturedRunID: "run-public",
			Runs: map[string]wallSpectatorRunControl{
				"run-hidden": {Visible: false},
			},
		}) //nolint:errcheck
	}))
	defer wall.Close()

	next := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		json.NewEncoder(res).Encode(map[string]any{
			"now": 123,
			"runs": []map[string]any{
				{"run_id": "run-hidden", "status": "running"},
				{"run_id": "run-public", "status": "running"},
				{"run_id": "run-finished", "status": "done"},
			},
			"summary": map[string]int{"live": 2, "completed": 1},
		}) //nolint:errcheck
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/watch", nil)
	res := httptest.NewRecorder()
	spectatorVisibilityHTTPHandler(wall.URL, next).ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.Code, res.Body.String())
	}

	var got struct {
		Runs []struct {
			RunID    string `json:"run_id"`
			Featured bool   `json:"featured"`
		} `json:"runs"`
		Summary       spectatorSummary `json:"summary"`
		FeaturedRunID string           `json:"featured_run_id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Runs) != 2 {
		t.Fatalf("runs = %+v, want 2 visible runs", got.Runs)
	}
	if got.Runs[0].RunID != "run-public" || !got.Runs[0].Featured {
		t.Fatalf("first visible run = %+v, want featured public run", got.Runs[0])
	}
	if got.FeaturedRunID != "run-public" {
		t.Fatalf("featured_run_id = %q", got.FeaturedRunID)
	}
	if got.Summary.Live != 1 || got.Summary.Completed != 1 || got.Summary.Queued != 0 {
		t.Fatalf("summary = %+v", got.Summary)
	}
}

func TestSpectatorVisibilityBlocksHiddenRunDirectReads(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		json.NewEncoder(res).Encode(wallSpectatorControl{
			Runs: map[string]wallSpectatorRunControl{"run-hidden": {Visible: false}},
		}) //nolint:errcheck
	}))
	defer wall.Close()

	called := false
	next := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		called = true
		res.WriteHeader(http.StatusOK)
	})
	handler := spectatorVisibilityHTTPHandler(wall.URL, next)

	for _, target := range []string{
		"/frame?run=run-hidden",
		"/v1/watch/runs/run-hidden/replay/status",
		"/v1/watch/runs/run-hidden/replay/video",
	} {
		called = false
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, target, nil))
		if res.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", target, res.Code)
		}
		if called {
			t.Fatalf("%s reached public downstream handler", target)
		}
	}
}

func TestSpectatorVisibilityFailsClosedWhenControlUnavailable(t *testing.T) {
	handler := spectatorVisibilityHTTPHandler("http://127.0.0.1:1", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("downstream handler must not run without visibility policy")
	}))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/watch", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.Code)
	}
}
