package operatorapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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

// The Telegram bot and the admin/MCP control plane perform the same two
// mutating operator actions. They must not drift onto separate endpoints or
// separate error handling, so both spellings of each action are asserted here.
func TestControlOperationsShareEndpointAndReturnBody(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/runs/run-1/cancel":
			_ = json.NewEncoder(w).Encode(map[string]any{"cancel": true, "run_id": "run-1"})
		case "/v1/triage/deadbeef/investigate":
			_ = json.NewEncoder(w).Encode(map[string]any{"issue_number": 42})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := New(srv.URL, "", "")
	body, err := client.CancelRun(context.Background(), " run-1 ")
	if err != nil {
		t.Fatal(err)
	}
	if body["cancel"] != true || body["run_id"] != "run-1" {
		t.Fatalf("cancel body = %v", body)
	}
	if err := client.Stop(context.Background(), "run-1"); err != nil {
		t.Fatal(err)
	}
	investigated, err := client.InvestigateFailure(context.Background(), "deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if investigated["issue_number"] != float64(42) {
		t.Fatalf("investigate body = %v", investigated)
	}
	if err := client.Investigate(context.Background(), "deadbeef"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"POST /v1/runs/run-1/cancel",
		"POST /v1/runs/run-1/cancel",
		"POST /v1/triage/deadbeef/investigate",
		"POST /v1/triage/deadbeef/investigate",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestControlOperationsSurfaceWallErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"run is not cancellable"}`, http.StatusConflict)
	}))
	defer srv.Close()

	client := New(srv.URL, "", "")
	if _, err := client.CancelRun(context.Background(), "run-1"); err == nil {
		t.Fatal("cancel of a rejected run reported success")
	}
	if err := client.Stop(context.Background(), "run-1"); err == nil {
		t.Fatal("stop of a rejected run reported success")
	}
}

func TestResolvedAlertsRequestsNonActiveAlerts(t *testing.T) {
	var mu sync.Mutex
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		query = r.URL.RawQuery
		mu.Unlock()
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"status":      map[string]any{"state": "suppressed"},
			"labels":      map[string]string{"alertname": "FarmDown"},
			"annotations": map[string]string{"summary": "farm recovered"},
			"fingerprint": "abc",
		}})
	}))
	defer srv.Close()

	alerts, err := New("", "", srv.URL).ResolvedAlerts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || alerts[0].Labels["alertname"] != "FarmDown" {
		t.Fatalf("resolved alerts = %+v", alerts)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{"active=false", "silenced=false", "inhibited=false", "unprocessed=false"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query %q missing %q", query, want)
		}
	}
}

func TestResolvedAlertsRequiresAlertmanager(t *testing.T) {
	if _, err := New("", "", "").ResolvedAlerts(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestDashboardNamesAnOversizedBody(t *testing.T) {
	// One JSON value larger than the operator decode cap. A mid-value cut used
	// to surface as "unexpected EOF", which the Telegram poll reports as the
	// wall being down.
	payload := []byte(`{"now":1,"runs":[{"run_id":"` + strings.Repeat("x", 4<<20) + `"}]}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "", "").Dashboard(context.Background(), true, 5)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v, want a size-cap error", err)
	}
	if strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("oversized body reported as a transport EOF: %v", err)
	}
}
