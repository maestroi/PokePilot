package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestArchiveDashboardFiltersAndSortsBeforePagination(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("limit"); got != "" {
			t.Fatalf("fallback limit = %q, want stripped for full-set sort", got)
		}
		if got := r.URL.Query().Get("offset"); got != "" {
			t.Fatalf("fallback offset = %q, want stripped for full-set sort", got)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"now":   1000,
			"total": 3,
			"runs": []any{
				map[string]any{
					"run_id": "slow-success", "status": "done", "reason": "done", "goal": "Earn the Boulder Badge", "play_style": "speedrun",
					"frame": 900.0, "ended_at": 900.0,
					"inference":      map[string]any{"model_id": "qwen-9b", "deployment_id": "qwen-9b-4090", "compute": "RTX 4090"},
					"llm_deployment": "qwen-9b-4090",
					"stats":          map[string]any{"rounds": 12.0, "strategic_seconds": 8.0, "goal_complete": true},
				},
				map[string]any{
					"run_id": "fast-success", "status": "done", "reason": "done", "goal": "Earn the Boulder Badge", "play_style": "speedrun",
					"frame": 300.0, "ended_at": 800.0,
					"inference":      map[string]any{"model_id": "qwen-4b", "deployment_id": "qwen-4b-4090", "compute": "RTX 4090"},
					"llm_deployment": "qwen-4b-4090",
					"stats":          map[string]any{"rounds": 18.0, "strategic_seconds": 5.0, "goal_complete": true},
				},
				map[string]any{
					"run_id": "failed", "status": "done", "reason": "budget", "goal": "Earn the Boulder Badge", "play_style": "adventure",
					"frame": 100.0, "ended_at": 700.0,
					"inference":      map[string]any{"model_id": "qwen-4b", "deployment_id": "qwen-4b-4090", "compute": "RTX 4090"},
					"llm_deployment": "qwen-4b-4090",
					"stats":          map[string]any{"rounds": 20.0, "strategic_seconds": 6.0, "goal_complete": false},
				},
			},
			"history_facets": map[string]any{"outcomes": []string{"budget", "done"}, "hows": []string{"play"}, "starters": []string{"squirtle"}},
		})
	})

	handler := archiveHTTPHandler(NewWall(""), fallback)
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard?status=done&goal=Earn+the+Boulder+Badge&success=1&sort=frames&direction=asc&limit=1&offset=0&facets=1", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var got struct {
		Total  int              `json:"total"`
		Runs   []map[string]any `json:"runs"`
		Facets struct {
			Models     []string `json:"models"`
			PlayStyles []string `json:"play_styles"`
		} `json:"history_facets"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 {
		t.Fatalf("total = %d, want 2 successful runs", got.Total)
	}
	if len(got.Runs) != 1 || got.Runs[0]["run_id"] != "fast-success" {
		t.Fatalf("runs = %#v, want fastest successful run first and paged", got.Runs)
	}
	if len(got.Facets.Models) != 2 || len(got.Facets.PlayStyles) != 2 {
		t.Fatalf("archive facets = %#v", got.Facets)
	}
}

func TestArchiveDashboardFiltersModelDeploymentExperimentAndCompute(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"now":   1000,
			"total": 2,
			"runs": []any{
				map[string]any{
					"run_id": "a", "status": "done", "reason": "done", "experiment_id": "exp-1", "frame": 100.0,
					"llm_deployment": "qwen-9b-4090", "inference": map[string]any{"model_id": "qwen-9b", "compute": "RTX 4090"},
				},
				map[string]any{
					"run_id": "b", "status": "done", "reason": "done", "experiment_id": "exp-2", "frame": 200.0,
					"llm_deployment": "qwen-4b-cpu", "inference": map[string]any{"model_id": "qwen-4b", "compute": "CPU"},
				},
			},
		})
	})
	handler := archiveHTTPHandler(NewWall(""), fallback)
	values := url.Values{
		"status":     {"done"},
		"model":      {"qwen-9b"},
		"deployment": {"qwen-9b-4090"},
		"compute":    {"RTX 4090"},
		"experiment": {"exp-1"},
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/dashboard?"+values.Encode(), nil))
	var got struct {
		Total int              `json:"total"`
		Runs  []map[string]any `json:"runs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Runs) != 1 || got.Runs[0]["run_id"] != "a" {
		t.Fatalf("filtered dashboard = total %d runs %#v", got.Total, got.Runs)
	}
}

func TestArchiveLeaseCapturesExecutionStartAndRuntime(t *testing.T) {
	var controller *archiveController
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/lease":
			writeJSON(w, http.StatusOK, farm.Spec{RunID: "runtime-run", Attempt: 1})
		case "/v1/dashboard":
			started := controller.startedAt("runtime-run")
			writeJSON(w, http.StatusOK, map[string]any{
				"now":   started + 10,
				"total": 1,
				"runs":  []any{map[string]any{"run_id": "runtime-run", "status": "done", "reason": "done", "ended_at": float64(started + 10)}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	controller = archiveHTTPHandler(NewWall(""), fallback).(*archiveController)

	leaseRes := httptest.NewRecorder()
	controller.ServeHTTP(leaseRes, httptest.NewRequest(http.MethodPost, "/v1/lease", nil))
	started := controller.startedAt("runtime-run")
	if started <= 0 {
		t.Fatal("lease did not record started_at")
	}

	dashboardRes := httptest.NewRecorder()
	controller.ServeHTTP(dashboardRes, httptest.NewRequest(http.MethodGet, "/v1/dashboard?status=done", nil))
	var got struct {
		Runs []struct {
			StartedAt      int64   `json:"started_at"`
			RuntimeSeconds float64 `json:"runtime_seconds"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(dashboardRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Runs) != 1 || got.Runs[0].StartedAt != started || got.Runs[0].RuntimeSeconds != 10 {
		t.Fatalf("runtime row = %#v, started %d", got.Runs, started)
	}
}
