package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestGracefulDrainRequeuesWithoutSpendingRecoveryBudgets(t *testing.T) {
	w := NewWall("")
	tile := &Tile{
		RunID: "drain-run", Status: statusRunning, Seed: 4242,
		RecoveryProfile:  farm.RecoveryProfileResilient,
		ErrorAttempts:    2,
		LossRecoveries:   3,
		RecoveryAttempts: 4,
	}
	w.tiles[tile.RunID] = tile

	w.mu.Lock()
	completed := w.settleRun(tile, "drained", "runner shutdown requested; stopped at safe objective boundary", time.Now())
	w.mu.Unlock()

	if completed != 1 || tile.Attempts != 1 {
		t.Fatalf("attempts = completed %d stored %d, want 1/1", completed, tile.Attempts)
	}
	if tile.Finished || tile.Status != statusQueued {
		t.Fatalf("drained run = status %q finished=%v, want queued/false", tile.Status, tile.Finished)
	}
	if tile.ErrorAttempts != 2 || tile.LossRecoveries != 3 || tile.RecoveryAttempts != 4 {
		t.Fatalf("drain spent recovery budgets: errors=%d losses=%d recovery=%d, want 2/3/4",
			tile.ErrorAttempts, tile.LossRecoveries, tile.RecoveryAttempts)
	}
	if tile.Seed != 4242 {
		t.Fatalf("drain changed seed to %d, want 4242", tile.Seed)
	}
	if !strings.HasPrefix(tile.Detail, "attempt 1 drained: ") {
		t.Fatalf("retry detail = %q, want drained marker", tile.Detail)
	}
	if len(w.queue) != 1 || w.queue[0] != tile.RunID {
		t.Fatalf("queue = %v, want [%s]", w.queue, tile.RunID)
	}
}

func TestGracefulDrainResumeUsesFlushedObjectivePair(t *testing.T) {
	w := NewWall(t.TempDir())
	const runID = "drain-resume"
	w.tiles[runID] = &Tile{
		RunID: runID, Status: statusQueued, Planner: "llm", Attempts: 1,
		Detail: "attempt 1 drained: deploy",
	}

	dir := checkpointAttemptDir(w.dumpsDir, runID, 1)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateName := "round-002-frame-0000000200-cancel.state"
	base := strings.TrimSuffix(stateName, ".state")
	if err := os.WriteFile(filepath.Join(dir, stateName), []byte("safe-state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+".knowledge-v4.json"), []byte(`{"intent":"continue"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	res := httptest.NewRecorder()
	w.handleCheckpointResume(res, runID, 2)
	if res.Code != http.StatusOK {
		t.Fatalf("resume status = %d body=%s", res.Code, res.Body.String())
	}
	var cp farm.ResumeCheckpoint
	if err := json.NewDecoder(res.Body).Decode(&cp); err != nil {
		t.Fatal(err)
	}
	if cp.State.Name != stateName || string(cp.State.Data) != "safe-state" {
		t.Fatalf("resume state = %q %q, want flushed objective pair", cp.State.Name, cp.State.Data)
	}
	if cp.Knowledge == nil || cp.Knowledge.Name != base+".knowledge-v4.json" {
		t.Fatalf("resume knowledge = %+v, want matching pair", cp.Knowledge)
	}
}
