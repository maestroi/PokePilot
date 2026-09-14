package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOutcomesStatsHandlerUsesNarrowWallFeed(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/outcomes" {
			t.Fatalf("path=%s, want /v1/outcomes", req.URL.Path)
		}
		res.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(res).Encode(map[string]any{"runs": []statsRun{
			{RunID: "done-1", Status: "done", Attempts: 1, Reason: "goal", Player: &statsPlayer{Badges: []string{"boulder"}}},
			{RunID: "done-2", Status: "done", Attempts: 2, Reason: "error"},
		}})
	}))
	defer wall.Close()

	res := httptest.NewRecorder()
	outcomesStatsHandler(wall.URL).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/stats", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var got farmOutcomeStats
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SettledRuns != 2 || got.CompletedAttempts != 3 || got.AtLeastOneBadge != 1 || got.BestBadges != 1 {
		t.Fatalf("stats=%#v", got)
	}
}

func TestOutcomesStatsHandlerCachesWallScan(t *testing.T) {
	var calls int32
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&calls, 1)
		_ = json.NewEncoder(res).Encode(map[string]any{"runs": []statsRun{
			{RunID: "done-1", Status: "done", Attempts: 1, Reason: "goal"},
		}})
	}))
	defer wall.Close()

	handler := outcomesStatsHandlerWithPolicy(wall.URL, time.Second, time.Minute)
	for i := 0; i < 2; i++ {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/stats", nil))
		if res.Code != http.StatusOK {
			t.Fatalf("request %d status=%d body=%s", i+1, res.Code, res.Body.String())
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("outcomes calls=%d, want 1", got)
	}
}

func TestOutcomesStatsHandlerServesStaleCacheWhenWallFails(t *testing.T) {
	var calls int32
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if atomic.AddInt32(&calls, 1) > 1 {
			http.Error(res, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(res).Encode(map[string]any{"runs": []statsRun{
			{RunID: "done-1", Status: "done", Attempts: 1, Reason: "goal"},
		}})
	}))
	defer wall.Close()

	handler := outcomesStatsHandlerWithPolicy(wall.URL, time.Second, 0)
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/v1/stats", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/v1/stats", nil))
	if second.Code != http.StatusOK {
		t.Fatalf("second status=%d body=%s", second.Code, second.Body.String())
	}
	if second.Header().Get("X-PokePilot-Stats-Stale") != "true" {
		t.Fatalf("stale header=%q", second.Header().Get("X-PokePilot-Stats-Stale"))
	}
	if second.Body.String() != first.Body.String() {
		t.Fatalf("stale body=%q, want cached %q", second.Body.String(), first.Body.String())
	}
}
