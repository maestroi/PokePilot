package operatorapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestRestartPrefersNewestReplayableCheckpointAndCancelsOriginal(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/run-1/checkpoints":
			_ = json.NewEncoder(w).Encode(map[string]any{"checkpoints": []map[string]any{
				{"name": "older.state", "frame": 100, "replayable": true},
				{"name": "newest.state", "frame": 200, "replayable": true},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/run-1/repro":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["checkpoint"] != "newest.state" {
				t.Fatalf("checkpoint = %v, want newest.state", in["checkpoint"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"run_id": "replay-2"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/run-1/cancel":
			_ = json.NewEncoder(w).Encode(map[string]any{"cancel": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := New(srv.URL, "", "")
	got, err := client.Restart(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != "replay-2" || got.Method != "checkpoint" || got.Checkpoint != "newest.state" {
		t.Fatalf("restart = %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("calls = %v", calls)
	}
}

func TestRestartFallsBackToFreshClone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/run-1/checkpoints":
			_ = json.NewEncoder(w).Encode(map[string]any{"checkpoints": []any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/run-1/clone":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"run_id": "run-2"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/run-1/cancel":
			_ = json.NewEncoder(w).Encode(map[string]any{"cancel": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	got, err := New(srv.URL, "", "").Restart(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != "run-2" || got.Method != "fresh-clone" {
		t.Fatalf("restart = %+v", got)
	}
}

func TestFrameAndAlerts(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/frame":
			if r.URL.Query().Get("run") != "abc 123" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		case "/v1/media/render-jobs":
			if r.URL.Query().Get("limit") != "5" {
				t.Fatalf("render job limit = %q", r.URL.Query().Get("limit"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []map[string]any{{"id": "render-1", "run_id": "run-1", "state": "ready", "mode": "raw"}}, "total": 1})
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	alertmanager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"status":      map[string]any{"state": "active"},
			"labels":      map[string]string{"alertname": "FarmDown"},
			"annotations": map[string]string{"summary": "farm unavailable"},
			"fingerprint": "abc",
		}})
	}))
	defer alertmanager.Close()

	client := New(wall.URL, "", alertmanager.URL)
	data, mediaType, err := client.Frame(context.Background(), "abc 123")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "png" || mediaType != "image/png" {
		t.Fatalf("frame = %q %q", data, mediaType)
	}
	alerts, err := client.Alerts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || alerts[0].Labels["alertname"] != "FarmDown" {
		t.Fatalf("alerts = %+v", alerts)
	}
	jobs, err := client.MediaRenderJobs(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if jobs.Total != 1 || len(jobs.Jobs) != 1 || jobs.Jobs[0].State != "ready" {
		t.Fatalf("render jobs = %+v", jobs)
	}
}
