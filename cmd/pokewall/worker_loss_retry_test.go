package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestWallWorkerLossDoesNotConsumeErrorBudgetAcrossRestart(t *testing.T) {
	const runID = "rollover"
	stateFile := filepath.Join(t.TempDir(), "wall-state.json")
	ctx := context.Background()

	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())
	client1 := farm.NewClient(srv1.URL)
	enqueueViaHTTP(t, srv1.URL, farm.Spec{RunID: runID, Planner: "scripted", Starter: "squirtle", Dest: "pallet"})

	spec, err := client1.Lease(ctx)
	if err != nil || spec == nil || spec.Attempt != 1 {
		t.Fatalf("lease 1 = %v, %v; want attempt 1", spec, err)
	}

	// Four deployment/worker rollovers used to exhaust the shared three-attempt
	// budget. They should now only advance generation identity and loss budget.
	for loss := 1; loss <= 4; loss++ {
		w1.mu.Lock()
		w1.tiles[runID].lastUpdate = time.Now().Add(-w1.staleAfter - time.Second)
		w1.mu.Unlock()
		if got := w1.reapStale(time.Now()); len(got) != 1 || got[0] != runID {
			t.Fatalf("loss %d reap = %v, want [%s]", loss, got, runID)
		}
		spec, err = client1.Lease(ctx)
		wantAttempt := loss + 1
		if err != nil || spec == nil || spec.Attempt != wantAttempt {
			t.Fatalf("lease after loss %d = %v, %v; want attempt %d", loss, spec, err, wantAttempt)
		}
	}

	w1.mu.Lock()
	before := *w1.tiles[runID]
	w1.mu.Unlock()
	if before.Attempts != 4 || before.LossRecoveries != 4 || before.ErrorAttempts != 0 || before.Finished {
		t.Fatalf("before restart = attempts %d losses %d errors %d finished %v; want 4/4/0/false",
			before.Attempts, before.LossRecoveries, before.ErrorAttempts, before.Finished)
	}
	srv1.Close()

	// The counters are part of wall state; a wall rollout must not reset either
	// budget or generation identity.
	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	w2.mu.Lock()
	restored := *w2.tiles[runID]
	w2.mu.Unlock()
	if restored.Attempts != 4 || restored.LossRecoveries != 4 || restored.ErrorAttempts != 0 || restored.Status != statusLeased {
		t.Fatalf("restored = status %s attempts %d losses %d errors %d; want leased/4/4/0",
			restored.Status, restored.Attempts, restored.LossRecoveries, restored.ErrorAttempts)
	}

	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()
	client2 := farm.NewClient(srv2.URL)

	// A zombie from generation 4 is still rejected now that generation 5 is
	// current, even though losses no longer consume the error retry budget.
	late := `{"run_id":"rollover","reason":"done","attempt":4}`
	resp, err := http.Post(srv2.URL+"/v1/runs/"+runID+"/finish", "application/json", strings.NewReader(late))
	if err != nil {
		t.Fatalf("POST stale finish: %v", err)
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // test probe
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale finish = %d, want 409", resp.StatusCode)
	}

	// Genuine failures still get exactly the historical three tries. They are
	// counted independently even after the four worker losses above.
	for errorAttempt := 1; errorAttempt <= maxAttempts; errorAttempt++ {
		if err := client2.Finish(ctx, farm.FinishReport{
			RunID: runID, Attempt: spec.Attempt, Reason: "error", Detail: "gameplay failure",
		}); err != nil {
			t.Fatalf("finish error %d: %v", errorAttempt, err)
		}
		if errorAttempt < maxAttempts {
			spec, err = client2.Lease(ctx)
			wantGeneration := 5 + errorAttempt
			if err != nil || spec == nil || spec.Attempt != wantGeneration {
				t.Fatalf("lease after error %d = %v, %v; want generation %d", errorAttempt, spec, err, wantGeneration)
			}
		}
	}

	w2.mu.Lock()
	final := *w2.tiles[runID]
	w2.mu.Unlock()
	if !final.Finished || final.Reason != "error" || final.Attempts != 7 || final.LossRecoveries != 4 || final.ErrorAttempts != maxAttempts {
		t.Fatalf("final = finished %v reason %s attempts %d losses %d errors %d; want true/error/7/4/%d",
			final.Finished, final.Reason, final.Attempts, final.LossRecoveries, final.ErrorAttempts, maxAttempts)
	}
	if next, err := client2.Lease(ctx); err != nil || next != nil {
		t.Fatalf("lease after error budget exhausted = %v, %v; want 204", next, err)
	}
}

func TestWallWorkerLossRecoveryHasSafetyCap(t *testing.T) {
	w := NewWall("")
	tile := &Tile{RunID: "crash-loop", Status: statusRunning}
	w.tiles[tile.RunID] = tile

	for loss := 1; loss <= maxLostRecoveries; loss++ {
		w.mu.Lock()
		w.settleRun(tile, "lost", "worker vanished", time.Now())
		if loss < maxLostRecoveries {
			if tile.Finished || tile.Status != statusQueued {
				w.mu.Unlock()
				t.Fatalf("loss %d settled early: status=%s finished=%v", loss, tile.Status, tile.Finished)
			}
			// Simulate the queued retry being leased by the next worker without
			// involving HTTP; the budget behavior itself is what this test pins.
			w.queue = nil
			tile.Status = statusRunning
		}
		w.mu.Unlock()
	}

	if !tile.Finished || tile.Status != statusDone || tile.Reason != "lost" {
		t.Fatalf("loss cap did not settle crash loop: status=%s reason=%s finished=%v", tile.Status, tile.Reason, tile.Finished)
	}
	if tile.Attempts != maxLostRecoveries || tile.LossRecoveries != maxLostRecoveries || tile.ErrorAttempts != 0 {
		t.Fatalf("loss cap counters = attempts %d losses %d errors %d; want %d/%d/0",
			tile.Attempts, tile.LossRecoveries, tile.ErrorAttempts, maxLostRecoveries, maxLostRecoveries)
	}
}
