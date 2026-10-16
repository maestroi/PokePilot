package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/metrics"
)

func TestWallFailureMetricsOnlyCountAcceptedFinish(t *testing.T) {
	reg := metrics.Must(metrics.Options{Service: metrics.ServiceWall, Revision: "dev"})
	w := NewWall("")
	w.SetFailureMetrics(mustWallFailures(t, reg))
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	enqueueViaHTTP(t, srv.URL, farm.Spec{
		RunID: "observability-unique-run", Game: "pokemon-gold", Planner: "llm",
	})
	client := farm.NewClient(srv.URL)
	if lease, err := client.Lease(context.Background()); err != nil || lease == nil {
		t.Fatalf("lease = %+v, %v", lease, err)
	}

	artifact, err := farm.NewObjectiveFailureArtifact([]farm.ObjectiveFailure{{
		Objective: "move", Cause: "route_prerequisite_missing", Count: 4,
		RecoveredCount: 2, TerminalCount: 1, Blocking: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	report := farm.FinishReport{
		RunID: "observability-unique-run", Attempt: 1, Reason: "stuck",
		Artifacts: []farm.Artifact{artifact},
	}
	if err := client.Finish(context.Background(), report); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := client.Finish(context.Background(), report); err != nil {
		t.Fatalf("duplicate finish: %v", err)
	}
	body := scrapeMetrics(t, reg.Handler())
	for _, want := range []string{
		`pokepilot_run_failures_total{failure_type="prerequisite",game="pokemon-gold",outcome="recovered"} 2`,
		`pokepilot_run_failures_total{failure_type="prerequisite",game="pokemon-gold",outcome="terminal"} 1`,
		`pokepilot_run_failures_total{failure_type="prerequisite",game="pokemon-gold",outcome="unresolved"} 1`,
		`pokepilot_failure_recoveries_total{failure_type="prerequisite",game="pokemon-gold"} 2`,
		`pokepilot_failure_time_to_failure_seconds_count{game="pokemon-gold",reason="stuck"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "observability-unique-run") || strings.Contains(body, "route_prerequisite_missing") {
		t.Fatalf("unbounded data leaked into Prometheus:\n%s", body)
	}
}

func TestWallFailureMetricsCountRetryWithoutTerminating(t *testing.T) {
	reg := metrics.Must(metrics.Options{Service: metrics.ServiceWall, Revision: "dev"})
	w := NewWall("")
	w.SetFailureMetrics(mustWallFailures(t, reg))
	tile := &Tile{RunID: "retry", Game: "pokemon-red", Status: statusRunning}
	w.mu.Lock()
	w.tiles[tile.RunID] = tile
	w.settleRun(tile, "error", "runner temporarily stopped", time.Now())
	w.mu.Unlock()
	body := scrapeMetrics(t, reg.Handler())
	if !strings.Contains(body, `pokepilot_failure_recovery_attempts_total{game="pokemon-red",reason="error"} 1`) {
		t.Fatalf("retry not counted:\n%s", body)
	}
	if strings.Contains(body, "pokepilot_failure_time_to_failure_seconds_count") {
		t.Fatalf("nonterminal retry was timed as a terminal failure:\n%s", body)
	}
}

func mustWallFailures(t *testing.T, reg *metrics.Registry) *metrics.Failures {
	t.Helper()
	f, err := metrics.NewFailures(reg)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
