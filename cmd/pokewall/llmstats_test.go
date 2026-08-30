package main

// The console's Play panel is the wall's data: heartbeats carry the tally,
// the dashboard serves it, a retry starts a fresh one, a finished run keeps
// its final one, and a restarted wall still has it.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func sampleStats(round int) *farm.LLMStats {
	return &farm.LLMStats{
		Round: round, RoundsLeft: 32 - round, Calls: round, Rounds: round,
		Repeats: 1, AvgOffered: 5.5, LastSeconds: 4.4, AvgSeconds: 3.1,
		PromptTokens: 947, CompletionTokens: 36,
		Intent: "get a move on the badge", IntentAge: 2,
		Choices: []farm.ChoiceCount{{Objective: "go to pallet town", Count: round}},
	}
}

func getDashboardView(t *testing.T, url string) dashboardView {
	t.Helper()
	res, err := http.Get(url + "/v1/dashboard")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	defer res.Body.Close()
	var dash dashboardView
	if err := json.NewDecoder(res.Body).Decode(&dash); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	return dash
}

func TestDashboardCarriesLLMStats(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "llm1", Planner: "llm", Goal: "Earn the Boulder Badge."})

	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease = %v, %v; want a spec", spec, err)
	}

	// Before the first ask there is no tally, and the dashboard says so.
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 1}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	dash := getDashboardView(t, srv.URL)
	if len(dash.Runs) != 1 || dash.Runs[0].Stats != nil {
		t.Fatalf("pre-first-ask stats = %+v, want nil", dash.Runs)
	}

	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 2, Stats: sampleStats(3)}); err != nil {
		t.Fatalf("heartbeat with stats: %v", err)
	}
	dash = getDashboardView(t, srv.URL)
	s := dash.Runs[0].Stats
	if s == nil || s.Round != 3 || s.Repeats != 1 || len(s.Choices) != 1 || s.Choices[0].Objective != "go to pallet town" {
		t.Fatalf("dashboard stats = %+v, want round 3 with the choice tally", s)
	}
}

func TestWallResetsStatsOnRetryKeptOnDone(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "flaky-llm", Planner: "llm"})

	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease 1 = %v, %v; want a spec", spec, err)
	}
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 5, Stats: sampleStats(7)}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: spec.RunID, Attempt: spec.Attempt, Reason: "error", Detail: "wild battle"}); err != nil {
		t.Fatalf("finish 1: %v", err)
	}
	w.mu.Lock()
	reset := w.tiles["flaky-llm"].Stats == nil && w.tiles["flaky-llm"].Status == statusQueued
	w.mu.Unlock()
	if !reset {
		t.Fatal("retried run kept attempt 1's stats")
	}

	// Attempt 2 finishes cleanly: the final tally is the interesting number.
	spec, err = client.Lease(ctx)
	if err != nil || spec == nil || spec.Attempt != 2 {
		t.Fatalf("lease 2 = %v, %v; want attempt 2", spec, err)
	}
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 9, Stats: sampleStats(12)}); err != nil {
		t.Fatalf("heartbeat 2: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: spec.RunID, Attempt: spec.Attempt, Reason: "done"}); err != nil {
		t.Fatalf("finish 2: %v", err)
	}
	w.mu.Lock()
	t2 := w.tiles["flaky-llm"]
	kept := t2.Finished && t2.Status == statusDone && t2.Stats != nil && t2.Stats.Round == 12
	w.mu.Unlock()
	if !kept {
		t.Fatalf("finished run did not keep its final stats (stats=%+v)", t2.Stats)
	}
}

func TestWallStatePersistsLLMStats(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "wall-state.json")
	ctx := context.Background()

	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())
	enqueueViaHTTP(t, srv1.URL, farm.Spec{RunID: "persist-llm", Planner: "llm"})
	c1 := farm.NewClient(srv1.URL)
	spec, err := c1.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease = %v, %v; want a spec", spec, err)
	}
	if _, err := c1.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 3, Stats: sampleStats(5)}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	srv1.Close()

	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()

	dash := getDashboardView(t, srv2.URL)
	if len(dash.Runs) != 1 || dash.Runs[0].Stats == nil || dash.Runs[0].Stats.Round != 5 {
		t.Fatalf("restored dashboard dropped the stats: %+v", dash.Runs)
	}
}
