package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/metrics"
)

func TestRunMetricsTerminalStatesAreIdempotent(t *testing.T) {
	reg := metrics.Must(metrics.Options{Service: metrics.ServiceWall, Revision: "dev"})
	w := NewWall("")
	w.SetRunMetrics(metrics.MustRuns(reg))
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()

	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{
		RunID: "metrics-done", Game: "pokemon-red", Planner: "scripted", Starter: "squirtle",
	})
	client := farm.NewClient(srv.URL)
	if got, err := client.Lease(ctx); err != nil || got == nil || got.RunID != "metrics-done" {
		t.Fatalf("lease = %+v, %v", got, err)
	}
	body := scrapeMetrics(t, reg.Handler())
	if !strings.Contains(body, `pokepilot_runs_active{game="pokemon-red"} 1`) {
		t.Fatalf("active after lease:\n%s", body)
	}

	hb := farm.Heartbeat{
		RunID: "metrics-done", Frame: 12000,
		Stats: &farm.LLMStats{Round: 3, Rounds: 3},
		Player: &farm.Player{
			Party:      []farm.PartyMon{{Name: "SQUIRTLE", Level: 6}},
			Badges:     []string{"Boulder"},
			Milestones: []string{"Pokédex", "a one-off npc"},
		},
	}
	if _, err := client.Heartbeat(ctx, hb); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if _, err := client.Heartbeat(ctx, hb); err != nil {
		t.Fatalf("repeat heartbeat: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "metrics-done", Reason: "done", Detail: "arrived"}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "metrics-done", Reason: "done", Detail: "arrived"}); err != nil {
		t.Fatalf("duplicate finish: %v", err)
	}

	body = scrapeMetrics(t, reg.Handler())
	for _, want := range []string{
		`pokepilot_runs_started_total{game="pokemon-red",mode="scripted"} 1`,
		`pokepilot_runs_completed_total{game="pokemon-red",mode="scripted"} 1`,
		`pokepilot_run_milestones_total{game="pokemon-red",milestone="starter"} 1`,
		`pokepilot_run_milestones_total{game="pokemon-red",milestone="badge_1"} 1`,
		`pokepilot_run_milestones_total{game="pokemon-red",milestone="pokedex"} 1`,
		`pokepilot_run_milestones_total{game="pokemon-red",milestone="completed"} 1`,
		`pokepilot_runs_active{game="pokemon-red"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "a one-off npc") || strings.Contains(body, "metrics-done") {
		t.Fatalf("exposition leaked an unbounded label:\n%s", body)
	}
	if strings.Contains(body, "pokepilot_runs_failed_total") && strings.Contains(body, `pokepilot_runs_failed_total{game="pokemon-red",mode="scripted"} 1`) {
		t.Fatalf("success was also counted as a failure:\n%s", body)
	}
}

func TestRunMetricsRetryIsNotATerminalFailure(t *testing.T) {
	reg := metrics.Must(metrics.Options{Service: metrics.ServiceWall, Revision: "dev"})
	w := NewWall("")
	w.SetRunMetrics(metrics.MustRuns(reg))
	queued := time.Now().Add(-2 * time.Minute)
	tile := &Tile{
		RunID: "retry-run", Status: statusRunning, Game: "pokemon-blue", Planner: "llm",
		QueuedAt: queued, Frame: 50,
		Player: &farm.Player{Party: []farm.PartyMon{{Name: "SQUIRTLE", Level: 8}}, Badges: []string{"Boulder"}},
	}
	w.mu.Lock()
	w.tiles[tile.RunID] = tile
	w.markRunActiveLocked(tile)
	w.noteRunMilestonesLocked(tile, tile.Player)
	w.settleRun(tile, "error", "blocked", time.Now())
	// The retry clears the live player. The same badges on the next
	// heartbeat must not count the funnel again.
	tile.Status = statusRunning
	tile.Player = &farm.Player{Party: []farm.PartyMon{{Name: "SQUIRTLE", Level: 9}}, Badges: []string{"Boulder", "Cascade"}}
	w.markRunActiveLocked(tile)
	w.noteRunMilestonesLocked(tile, tile.Player)
	w.settleRun(tile, "done", "arrived", time.Now())
	w.mu.Unlock()

	body := scrapeMetrics(t, reg.Handler())
	for _, want := range []string{
		`pokepilot_runs_completed_total{game="pokemon-blue",mode="llm"} 1`,
		`pokepilot_run_milestones_total{game="pokemon-blue",milestone="starter"} 1`,
		`pokepilot_run_milestones_total{game="pokemon-blue",milestone="badge_1"} 1`,
		`pokepilot_run_milestones_total{game="pokemon-blue",milestone="badge_2"} 1`,
		`pokepilot_run_stopping_milestone_total{game="pokemon-blue",milestone="completed",result="completed"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, `pokepilot_runs_failed_total{game="pokemon-blue",mode="llm"}`) {
		t.Fatalf("retry was counted as a terminal failure:\n%s", body)
	}
}

func TestRunMetricsAbortAndRestartDoNotReplayCounters(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "wall-state.json")
	ended := time.Unix(1_700_000_000, 0)

	w1 := NewWall("")
	w1.SetStatePath(state)
	w1.mu.Lock()
	w1.order = []string{"live", "won"}
	w1.tiles["live"] = &Tile{RunID: "live", Status: statusRunning, Game: "pokemon-red", Planner: "llm"}
	w1.tiles["won"] = &Tile{
		RunID: "won", Status: statusDone, Game: "pokemon-red", Planner: "llm",
		Reason: "done", Finished: true, EndedAt: ended,
	}
	w1.mu.Unlock()
	w1.saveState()

	reg := metrics.Must(metrics.Options{Service: metrics.ServiceWall, Revision: "dev"})
	w2 := NewWall("")
	w2.SetRunMetrics(metrics.MustRuns(reg))
	w2.SetStatePath(state)

	body := scrapeMetrics(t, reg.Handler())
	if !strings.Contains(body, `pokepilot_runs_active{game="pokemon-red"} 1`) {
		t.Fatalf("restored active gauge:\n%s", body)
	}
	if strings.Contains(body, "pokepilot_runs_completed_total") || strings.Contains(body, "pokepilot_runs_started_total") {
		t.Fatalf("restart replayed counters:\n%s", body)
	}
	got := gaugeValue(t, body, `pokepilot_last_successful_run_timestamp_seconds{game="pokemon-red"}`)
	if got != float64(ended.Unix()) {
		t.Fatalf("last success = %v, want %d\n%s", got, ended.Unix(), body)
	}

	w2.mu.Lock()
	queued := w2.tiles["queued-cancel"]
	if queued == nil {
		queued = &Tile{RunID: "queued-cancel", Status: statusQueued, Game: "boxxle", Planner: "policy"}
		w2.tiles[queued.RunID] = queued
	}
	w2.settleRun(queued, "cancelled", "cancelled while queued", time.Now())
	w2.mu.Unlock()
	body = scrapeMetrics(t, reg.Handler())
	if !strings.Contains(body, `pokepilot_runs_aborted_total{game="boxxle",mode="policy"} 1`) {
		t.Fatalf("abort:\n%s", body)
	}
	if strings.Contains(body, `pokepilot_runs_active{game="boxxle"}`) {
		t.Fatalf("queued cancel changed the active gauge:\n%s", body)
	}
}

func TestRunMetricsCatalogRestoresLastSuccessAfterEviction(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "wall-state.json")
	catalogPath := filepath.Join(dir, "catalog.db")
	ended := time.Unix(1_700_000_100, 0)

	w := NewWall("")
	reg := metrics.Must(metrics.Options{Service: metrics.ServiceWall, Revision: "dev"})
	w.SetRunMetrics(metrics.MustRuns(reg))
	w.SetStatePath(state)
	w.mu.Lock()
	w.order = []string{"gold-win"}
	w.tiles["gold-win"] = &Tile{
		RunID: "gold-win", Status: statusDone, Game: "pokemon-gold", Planner: "llm",
		Reason: "done", Finished: true, EndedAt: ended,
	}
	w.mu.Unlock()
	w.saveState()
	if err := w.SetCatalogPath(catalogPath); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.tiles["gold-win"]; ok {
		t.Fatal("finished run stayed in RAM after catalog eviction")
	}
	body := scrapeMetrics(t, reg.Handler())
	if got := gaugeValue(t, body, `pokepilot_last_successful_run_timestamp_seconds{game="pokemon-gold"}`); got != float64(ended.Unix()) {
		t.Fatalf("last success after eviction = %v\n%s", got, body)
	}

	reg2 := metrics.Must(metrics.Options{Service: metrics.ServiceWall, Revision: "dev"})
	w2 := NewWall("")
	w2.SetRunMetrics(metrics.MustRuns(reg2))
	w2.SetStatePath(state)
	body = scrapeMetrics(t, reg2.Handler())
	if strings.Contains(body, `game="pokemon-gold"`) {
		t.Fatalf("evicted success was still in the state file:\n%s", body)
	}
	if err := w2.SetCatalogPath(catalogPath); err != nil {
		t.Fatal(err)
	}
	body = scrapeMetrics(t, reg2.Handler())
	if got := gaugeValue(t, body, `pokepilot_last_successful_run_timestamp_seconds{game="pokemon-gold"}`); got != float64(ended.Unix()) {
		t.Fatalf("catalog restore = %v\n%s", got, body)
	}
	if strings.Contains(body, "pokepilot_runs_completed_total") {
		t.Fatalf("catalog restore incremented completed:\n%s", body)
	}
}

func scrapeMetrics(t *testing.T, h http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	return string(body)
}

func gaugeValue(t *testing.T, body, sample string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, sample+" ") && line != sample {
			if !strings.HasPrefix(line, sample) {
				continue
			}
		}
		if !strings.HasPrefix(line, sample) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		v, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.Fatalf("parse %q: %v", line, err)
		}
		return v
	}
	t.Fatalf("sample %s not in exposition", sample)
	return 0
}
