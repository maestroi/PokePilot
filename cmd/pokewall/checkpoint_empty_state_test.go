package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

// TestCheckpointUploadRejectsEmptyState is the wall's write-side guard. An
// empty state is what a reader observes when it reads a save file between
// truncate and write, and storing one makes it the run's newest checkpoint.
func TestCheckpointUploadRejectsEmptyState(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "empty-state", Planner: "llm", Goal: farm.GoalFrom("beat the game")})

	// Build the report by hand: farm.Client.Checkpoint refuses it before the
	// request leaves a runner, so only a direct POST can exercise the wall.
	emptySHA := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	body, err := json.Marshal(map[string]any{
		"run_id":  "empty-state",
		"attempt": 1,
		"artifacts": []map[string]any{{
			"name":       "round-001-frame-0003842410-progress-secret-key-owned.state",
			"media_type": "application/octet-stream",
			"sha256":     emptySHA,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL+"/v1/runs/empty-state/checkpoint", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("checkpoint status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	dir := checkpointAttemptDir(w.dumpsDir, "empty-state", 1)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) != 0 {
		t.Fatalf("rejected checkpoint left %d file(s) behind in %s", len(entries), dir)
	}
}

// TestStoredCheckpointArtifactRejectsEmptyState covers the PostgreSQL/S3 resume
// branch: a stored state row that is empty must fail materialization so
// latestStoredPair skips to an older usable pair. The inline branch needs no
// database, and the object branch shares the same validation call.
func TestStoredCheckpointArtifactRejectsEmptyState(t *testing.T) {
	cp := &controlPlane{}
	emptySHA := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	empty := storedCheckpointArtifact{
		meta: farm.Artifact{
			Name:      "round-001-frame-0003842410-progress-secret-key-owned.state",
			MediaType: "application/octet-stream",
			SHA256:    emptySHA,
		},
		hasInline: true,
	}
	if _, err := cp.materializeStoredArtifact(empty); err == nil {
		t.Fatal("stored empty checkpoint state was materialized as a resume candidate")
	}

	payload := []byte("complete-emulator-state")
	sum := sha256.Sum256(payload)
	usable := storedCheckpointArtifact{
		meta: farm.Artifact{
			Name:      "round-002-frame-0000000200-goto.state",
			MediaType: "application/octet-stream",
			SHA256:    hex.EncodeToString(sum[:]),
		},
		inline:    payload,
		hasInline: true,
	}
	art, err := cp.materializeStoredArtifact(usable)
	if err != nil {
		t.Fatalf("usable stored checkpoint rejected: %v", err)
	}
	if string(art.Data) != string(payload) || art.Store != "" {
		t.Fatalf("materialized artifact = %+v", art)
	}
}

// TestLostWorkerRetrySkipsEmptyNewestState is the wall's read-side guard, and
// the regression for the poisoned line: resume candidates are ordered by the
// frame embedded in the checkpoint name, so an empty checkpoint carrying a
// high frame outranks every checkpoint the run produced afterwards. Serving it
// made the runner boot a fresh cartridge on every retry; the wall must skip it
// and offer the newest usable pair instead.
func TestLostWorkerRetrySkipsEmptyNewestState(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "poisoned", Planner: "llm", Goal: farm.GoalFrom("beat the game")})

	first, err := client.Lease(ctx)
	if err != nil || first == nil || first.Attempt != 1 {
		t.Fatalf("lease 1 = %+v, %v", first, err)
	}
	usableState := wallResumeArtifact("round-002-frame-0000000200-goto.state", []byte("usable-emulator-state"), "application/octet-stream")
	usableKnowledge := wallResumeArtifact("round-002-frame-0000000200-goto.knowledge-v4.json", []byte(`{"intent":"usable"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "poisoned", Attempt: 1,
		Artifacts: []farm.Artifact{usableState, usableKnowledge},
	}); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	// Simulate the poisoned lineage an older wall could still hold: a carried
	// pre-objective checkpoint whose state was published from a mid-write read.
	dir := checkpointAttemptDir(w.dumpsDir, "poisoned", 1)
	poisoned := "round-001-frame-0000100000-goto.state"
	if err := os.WriteFile(filepath.Join(dir, poisoned), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "round-001-frame-0000100000-goto.knowledge-v4.json"), []byte(`{"intent":"poisoned"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	w.mu.Lock()
	w.tiles["poisoned"].Status = statusRunning
	w.tiles["poisoned"].lastUpdate = time.Now().Add(-time.Minute)
	w.mu.Unlock()
	if got := w.reapStale(time.Now()); len(got) != 1 || got[0] != "poisoned" {
		t.Fatalf("reaped = %v", got)
	}

	second, err := client.Lease(ctx)
	if err != nil || second == nil || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "poisoned", second.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil {
		t.Fatal("wall offered no checkpoint: the run would restart from a fresh cartridge")
	}
	if cp.State.Name != usableState.Name {
		t.Fatalf("resume state = %q, want the newest usable pair %q", cp.State.Name, usableState.Name)
	}
	if string(cp.State.Data) != "usable-emulator-state" {
		t.Fatalf("resume state bytes = %q", cp.State.Data)
	}
	if cp.Knowledge == nil || cp.Knowledge.Name != usableKnowledge.Name {
		t.Fatalf("resume knowledge = %+v, want %q", cp.Knowledge, usableKnowledge.Name)
	}
}

// TestLostWorkerResumePrefersDeeperCheckpointFromEarlierAttempt covers the
// second half of the same defect: when worker loss falls back to the lineage, a
// run that booted fresh writes its own early checkpoints, and ordering recovery
// by attempt instead of by embedded frame let those shallow checkpoints outrank
// the deep progress an earlier attempt reached — recovery then replayed hours of
// gameplay. The frame is cumulative emulator progress, so it must win.
func TestLostWorkerResumePrefersDeeperCheckpointFromEarlierAttempt(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "deep", Planner: "llm", Goal: farm.GoalFrom("beat the game")})

	if first, err := client.Lease(ctx); err != nil || first == nil || first.Attempt != 1 {
		t.Fatalf("lease 1 = %+v, %v", first, err)
	}
	deepState := wallResumeArtifact("round-040-frame-0003000000-route-12.state", []byte("deep-progress"), "application/octet-stream")
	deepKnowledge := wallResumeArtifact("round-040-frame-0003000000-route-12.knowledge-v4.json", []byte(`{"intent":"deep"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "deep", Attempt: 1, Artifacts: []farm.Artifact{deepState, deepKnowledge},
	}); err != nil {
		t.Fatalf("checkpoint attempt 1: %v", err)
	}
	reapLostRun(t, w, "deep")

	// Attempt 2 boots fresh and writes only early checkpoints.
	if second, err := client.Lease(ctx); err != nil || second == nil || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	shallowState := wallResumeArtifact("round-001-frame-0000030000-go-to-route-1.state", []byte("fresh-boot"), "application/octet-stream")
	shallowKnowledge := wallResumeArtifact("round-001-frame-0000030000-go-to-route-1.knowledge-v4.json", []byte(`{"intent":"shallow"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "deep", Attempt: 2, Artifacts: []farm.Artifact{shallowState, shallowKnowledge},
	}); err != nil {
		t.Fatalf("checkpoint attempt 2: %v", err)
	}
	reapLostRun(t, w, "deep")

	// Attempt 3 is lost before it writes anything, which is the common shape of
	// worker loss under deployment churn: the lineage fallback takes over.
	if third, err := client.Lease(ctx); err != nil || third == nil || third.Attempt != 3 {
		t.Fatalf("lease 3 = %+v, %v", third, err)
	}
	reapLostRun(t, w, "deep")

	fourth, err := client.Lease(ctx)
	if err != nil || fourth == nil || fourth.Attempt != 4 {
		t.Fatalf("lease 4 = %+v, %v", fourth, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "deep", fourth.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil {
		t.Fatal("wall offered no checkpoint: the run would restart from a fresh cartridge")
	}
	if cp.State.Name != deepState.Name || cp.Attempt != 1 {
		t.Fatalf("resume = %s (attempt %d), want the deeper %s from attempt 1", cp.State.Name, cp.Attempt, deepState.Name)
	}
}

func reapLostRun(t *testing.T, w *Wall, runID string) {
	t.Helper()
	w.mu.Lock()
	w.tiles[runID].Status = statusRunning
	w.tiles[runID].lastUpdate = time.Now().Add(-time.Minute)
	w.mu.Unlock()
	if got := w.reapStale(time.Now()); len(got) != 1 || got[0] != runID {
		t.Fatalf("reaped = %v, want [%s]", got, runID)
	}
}
