package main

// A failed run is not a finished run: error and lost are retried up to
// maxAttempts, each retry with a fresh seed (the same seed replays the same
// bad luck), while done/stuck/budget and a user's cancel settle at once.
// A late finish from an earlier attempt must not settle a newer one.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestWallRetriesFailedRun(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "flaky", Planner: "scripted", Starter: "squirtle", Dest: "pallet"})

	// Attempt 1 fails: the run goes back in the queue with a fresh seed.
	spec, err := client.Lease(ctx)
	if err != nil || spec == nil || spec.Attempt != 1 {
		t.Fatalf("lease 1 = %v, %v; want attempt 1", spec, err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "flaky", Attempt: spec.Attempt, Reason: "error", Detail: "wild battle"}); err != nil {
		t.Fatalf("finish 1: %v", err)
	}
	w.mu.Lock()
	tile := w.tiles["flaky"]
	retried := tile.Status == statusQueued && tile.Attempts == 1 && tile.Seed != spec.Seed
	w.mu.Unlock()
	if !retried {
		t.Fatalf("run not retried after error (status=%s attempts=%d seed=%d)", tile.Status, tile.Attempts, tile.Seed)
	}

	// Attempts 2 and 3 fail too; the third is the last, so the run settles.
	for attempt := 2; attempt <= maxAttempts; attempt++ {
		spec, err = client.Lease(ctx)
		if err != nil || spec == nil || spec.Attempt != attempt {
			t.Fatalf("lease %d = %v, %v; want attempt %d", attempt, spec, err, attempt)
		}
		if err := client.Finish(ctx, farm.FinishReport{RunID: "flaky", Attempt: spec.Attempt, Reason: "error", Detail: "wild battle"}); err != nil {
			t.Fatalf("finish %d: %v", attempt, err)
		}
	}

	w.mu.Lock()
	tile = w.tiles["flaky"]
	settled := tile.Finished && tile.Status == statusDone && tile.Reason == "error" && tile.Attempts == maxAttempts
	w.mu.Unlock()
	if !settled {
		t.Fatalf("run did not settle after %d attempts (status=%s reason=%s attempts=%d)",
			maxAttempts, tile.Status, tile.Reason, tile.Attempts)
	}
	if spec, err := client.Lease(ctx); err != nil || spec != nil {
		t.Fatalf("lease after exhaustion = %v, %v; want 204", spec, err)
	}
}

func TestWallDoesNotRetrySettledReasons(t *testing.T) {
	for _, reason := range []string{"done", "stuck", "budget"} {
		t.Run(reason, func(t *testing.T) {
			w := NewWall("")
			srv := httptest.NewServer(w.Handler())
			defer srv.Close()
			ctx := context.Background()
			client := farm.NewClient(srv.URL)
			enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "r", Planner: "scripted", Starter: "squirtle", Dest: "pallet"})

			spec, err := client.Lease(ctx)
			if err != nil || spec == nil {
				t.Fatalf("lease: %v", err)
			}
			if err := client.Finish(ctx, farm.FinishReport{RunID: "r", Attempt: spec.Attempt, Reason: reason}); err != nil {
				t.Fatalf("finish: %v", err)
			}

			w.mu.Lock()
			tile := w.tiles["r"]
			w.mu.Unlock()
			if !tile.Finished || tile.Status != statusDone || tile.Reason != reason || tile.Attempts != 1 {
				t.Fatalf("reason %s settled wrong: status=%s reason=%s attempts=%d",
					reason, tile.Status, tile.Reason, tile.Attempts)
			}
			if spec, err := client.Lease(ctx); err != nil || spec != nil {
				t.Fatalf("lease after %s = %v, %v; want 204", reason, spec, err)
			}
		})
	}
}

func TestWallDoesNotRetryCancelledRun(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "c1", Planner: "scripted", Starter: "squirtle", Dest: "pallet"})

	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease: %v", err)
	}
	res, err := http.Post(srv.URL+"/v1/runs/c1/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck // test probe
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST cancel = %d, want 200", res.StatusCode)
	}

	// A cancelled run may well report "error"; the cancel flag still wins.
	if err := client.Finish(ctx, farm.FinishReport{RunID: "c1", Attempt: spec.Attempt, Reason: "error", Detail: "cancelled"}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	w.mu.Lock()
	tile := w.tiles["c1"]
	w.mu.Unlock()
	if !tile.Finished || tile.Status != statusDone || tile.Reason != "error" || tile.Attempts != 1 {
		t.Fatalf("cancelled run settled wrong: status=%s reason=%s attempts=%d",
			tile.Status, tile.Reason, tile.Attempts)
	}
	if spec, err := client.Lease(ctx); err != nil || spec != nil {
		t.Fatalf("lease after cancel = %v, %v; want 204 (a cancel is never retried)", spec, err)
	}
}

func TestWallStaleFinishIsConflict(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "z1", Planner: "scripted", Starter: "squirtle", Dest: "pallet"})

	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "z1", Attempt: spec.Attempt, Reason: "error", Detail: "runner died"}); err != nil {
		t.Fatalf("finish 1: %v", err)
	}

	// The zombie's duplicate finish for attempt 1 arrives after the retry:
	// it must conflict, not settle attempt 2.
	report := `{"run_id":"z1","reason":"done","attempt":1}`
	res, err := http.Post(srv.URL+"/v1/runs/z1/finish", "application/json", strings.NewReader(report))
	if err != nil {
		t.Fatalf("POST stale finish: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck // test probe
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("stale finish = %d, want 409", res.StatusCode)
	}

	w.mu.Lock()
	tile := w.tiles["z1"]
	w.mu.Unlock()
	if tile.Finished || tile.Status != statusQueued {
		t.Fatalf("stale finish clobbered the retry: status=%s finished=%v", tile.Status, tile.Finished)
	}
}
