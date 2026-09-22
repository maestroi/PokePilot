package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func queueRecoveryTestRun(t *testing.T, baseURL string, spec farm.Spec) {
	t.Helper()
	body := fmt.Sprintf(`{"run_id":%q,"planner":%q,"goal":%q,"recovery_profile":%q}`,
		spec.RunID, spec.Planner, spec.Goal, spec.RecoveryProfile)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/specs", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("queue run: status %d", res.StatusCode)
	}
}

func TestResilientGoalSurvivesRepeatedErrorBudget(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(pauseHTTPHandler(w, w.Handler()))
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()

	queueRecoveryTestRun(t, srv.URL, farm.Spec{
		RunID: "resilient-loop", Planner: "llm", Goal: "beat the game",
		RecoveryProfile: farm.RecoveryProfileResilient,
	})

	const failures = maxAttempts + 2
	for attempt := 1; attempt <= failures; attempt++ {
		spec, err := client.Lease(ctx)
		if err != nil || spec == nil {
			t.Fatalf("lease %d = %+v, %v", attempt, spec, err)
		}
		if spec.Attempt != attempt || spec.RecoveryProfile != farm.RecoveryProfileResilient {
			t.Fatalf("lease %d = attempt %d profile %q", attempt, spec.Attempt, spec.RecoveryProfile)
		}
		if err := client.Finish(ctx, farm.FinishReport{
			RunID: "resilient-loop", Attempt: attempt, Reason: "error",
			Detail: "same deterministic blocker",
		}); err != nil {
			t.Fatalf("finish %d: %v", attempt, err)
		}

		w.mu.Lock()
		tile := *w.tiles["resilient-loop"]
		w.mu.Unlock()
		if tile.Status != statusQueued || tile.Finished {
			t.Fatalf("after failure %d: status=%q finished=%v", attempt, tile.Status, tile.Finished)
		}
		if tile.RecoveryAttempts != attempt {
			t.Fatalf("after failure %d: recovery attempts=%d", attempt, tile.RecoveryAttempts)
		}
	}

	next, err := client.Lease(ctx)
	if err != nil || next == nil || next.Attempt != failures+1 {
		t.Fatalf("lease after legacy error budget = %+v, %v", next, err)
	}
	if err := client.Finish(ctx, farm.FinishReport{
		RunID: "resilient-loop", Attempt: next.Attempt, Reason: "done",
	}); err != nil {
		t.Fatalf("finish done: %v", err)
	}
	w.mu.Lock()
	tile := *w.tiles["resilient-loop"]
	w.mu.Unlock()
	if tile.Status != statusDone || !tile.Finished || tile.Reason != "done" {
		t.Fatalf("completed resilient run = status %q finished=%v reason=%q", tile.Status, tile.Finished, tile.Reason)
	}
}

func TestResilientGoalRequeuesRunLevelStops(t *testing.T) {
	for _, reason := range []string{"stuck", "failed", "budget"} {
		t.Run(reason, func(t *testing.T) {
			w := NewWall("")
			tile := &Tile{
				RunID: "r", Status: statusRunning, Planner: "llm", Goal: "beat the game",
				RecoveryProfile: farm.RecoveryProfileResilient,
			}
			w.tiles["r"] = tile
			w.settleRun(tile, reason, "blocked", time.Now())
			if tile.Status != statusQueued || tile.Finished {
				t.Fatalf("%s became terminal: status=%q finished=%v", reason, tile.Status, tile.Finished)
			}
			if tile.RecoveryAttempts != 1 {
				t.Fatalf("%s recovery attempts=%d, want 1", reason, tile.RecoveryAttempts)
			}
		})
	}
}

func TestStrictGoalKeepsRunLevelStopsTerminal(t *testing.T) {
	for _, reason := range []string{"stuck", "failed", "budget"} {
		t.Run(reason, func(t *testing.T) {
			w := NewWall("")
			tile := &Tile{RunID: "r", Status: statusRunning, Planner: "llm", Goal: "beat the game"}
			w.tiles["r"] = tile
			w.settleRun(tile, reason, "blocked", time.Now())
			if tile.Status != statusDone || !tile.Finished {
				t.Fatalf("%s did not preserve strict terminal behavior: status=%q finished=%v", reason, tile.Status, tile.Finished)
			}
		})
	}
}

func TestResilientRecoveryDepthResetsOnlyOnNewProgress(t *testing.T) {
	tile := &Tile{
		RecoveryProfile:  farm.RecoveryProfileResilient,
		RecoveryAttempts: 4, RecoveryBadges: 2, RecoveryEvents: 20, RecoveryMaps: 30,
	}
	noteRecoveryProgressLocked(tile, &farm.Progress{Badges: 2, Events: 20, Maps: 30})
	if tile.RecoveryAttempts != 4 {
		t.Fatalf("same frontier reset recovery depth to %d", tile.RecoveryAttempts)
	}
	noteRecoveryProgressLocked(tile, &farm.Progress{Badges: 2, Events: 21, Maps: 30})
	if tile.RecoveryAttempts != 0 || tile.RecoveryEvents != 21 {
		t.Fatalf("new frontier = attempts %d events %d", tile.RecoveryAttempts, tile.RecoveryEvents)
	}

	w := NewWall("")
	w.tiles["r"] = tile
	tile.RunID = "r"
	tile.Status = statusRunning
	w.settleRun(tile, "error", "next blocker", time.Now())
	if tile.RecoveryAttempts != 1 {
		t.Fatalf("first blocker after progress = recovery attempts %d, want 1", tile.RecoveryAttempts)
	}
}

func TestResilientMajorRollbackWalksBackwardThenFresh(t *testing.T) {
	w := NewWall(t.TempDir())
	w.tiles["r"] = &Tile{RunID: "r", Attempts: 1}
	dir := checkpointAttemptDir(w.dumpsDir, "r", 1)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for badge := 1; badge <= 3; badge++ {
		base := fmt.Sprintf("major-badge-%d-test", badge)
		if err := os.WriteFile(filepath.Join(dir, base+".state"), []byte(fmt.Sprintf("state-%d", badge)), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, base+".knowledge-v4.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for rollback, wantBadge := range map[int]int{0: 3, 1: 2, 2: 1} {
		cp, err := w.latestLineageMajorCheckpointRollback("r", rollback)
		if err != nil {
			t.Fatalf("rollback %d: %v", rollback, err)
		}
		got, ok := majorCheckpointBadge(cp.State.Name)
		if !ok || got != wantBadge {
			t.Fatalf("rollback %d -> %q badge %d, want %d", rollback, cp.State.Name, got, wantBadge)
		}
	}
	if _, err := w.latestLineageMajorCheckpointRollback("r", 3); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rollback past retained milestones = %v, want os.ErrNotExist for fresh boot", err)
	}
}
