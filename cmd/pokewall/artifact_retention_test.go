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
)

func writeRetentionFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeDeleteRemovesCheckpointTree(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	w.mu.Lock()
	w.order = []string{"run-clean"}
	w.tiles["run-clean"] = &Tile{RunID: "run-clean", Status: statusDone, Finished: true, Attempts: 2}
	w.mu.Unlock()

	for _, path := range localFinishDumpPaths(dir, "run-clean", 2) {
		writeRetentionFixture(t, path)
	}
	checkpoint := filepath.Join(checkpointAttemptDir(dir, "run-clean", 2), "round-001.state")
	writeRetentionFixture(t, checkpoint)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/run-clean", nil)
	req.SetPathValue("id", "run-clean")
	rec := httptest.NewRecorder()
	w.handleRuntimeDelete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "checkpoints", safeBase("run-clean"))); !os.IsNotExist(err) {
		t.Fatalf("checkpoint tree still exists: %v", err)
	}
}

func TestDeleteProtectsLiveResumeParent(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	w.mu.Lock()
	w.order = []string{"parent", "child"}
	w.tiles["parent"] = &Tile{RunID: "parent", Status: statusDone, Finished: true, Attempts: 1}
	w.tiles["child"] = &Tile{RunID: "child", Status: statusQueued, Endless: true, ResumeFromRunID: "parent"}
	w.mu.Unlock()
	checkpoint := filepath.Join(checkpointAttemptDir(dir, "parent", 1), "major-badge-1.state")
	writeRetentionFixture(t, checkpoint)
	writeRetentionFixture(t, filepath.Join(dir, safeDumpName("parent")))

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/parent", nil)
	req.SetPathValue("id", "parent")
	rec := httptest.NewRecorder()
	w.handleRuntimeDelete(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("runtime delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := w.snapshotRun("parent"); !ok {
		t.Fatal("protected resume parent was removed from wall state")
	}
	if _, err := os.Stat(checkpoint); err != nil {
		t.Fatalf("protected checkpoint was removed: %v", err)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/runs/parent", nil)
	req.SetPathValue("id", "parent")
	rec = httptest.NewRecorder()
	w.handleDelete(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("compat delete status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInspectAndDashboardMarkLiveResumeParents(t *testing.T) {
	w := NewWall("")
	w.mu.Lock()
	w.order = []string{"parent", "unrelated", "child"}
	w.tiles["parent"] = &Tile{RunID: "parent", Status: statusDone, Finished: true, Attempts: 1}
	w.tiles["unrelated"] = &Tile{RunID: "unrelated", Status: statusDone, Finished: true, Attempts: 1}
	w.tiles["child"] = &Tile{RunID: "child", Status: statusRunning, Endless: true, ResumeFromRunID: "parent"}
	w.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/parent", nil)
	req.SetPathValue("id", "parent")
	rec := httptest.NewRecorder()
	w.handleRunInspect(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect parent status=%d body=%s", rec.Code, rec.Body.String())
	}
	var parentView struct {
		DeleteBlocked string `json:"delete_blocked"`
		Run           struct {
			ResumeProtected bool `json:"resume_protected"`
		} `json:"run"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&parentView); err != nil {
		t.Fatal(err)
	}
	if parentView.DeleteBlocked == "" || !strings.Contains(parentView.DeleteBlocked, "resume lineage") {
		t.Fatalf("inspect parent delete_blocked=%q", parentView.DeleteBlocked)
	}
	if !parentView.Run.ResumeProtected {
		t.Fatal("inspect parent should be resume_protected")
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/runs/unrelated", nil)
	req.SetPathValue("id", "unrelated")
	rec = httptest.NewRecorder()
	w.handleRunInspect(rec, req)
	var otherView struct {
		DeleteBlocked string `json:"delete_blocked"`
		Run           struct {
			ResumeProtected bool `json:"resume_protected"`
		} `json:"run"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&otherView); err != nil {
		t.Fatal(err)
	}
	if otherView.DeleteBlocked != "" || otherView.Run.ResumeProtected {
		t.Fatalf("unrelated inspect blocked=%q protected=%v", otherView.DeleteBlocked, otherView.Run.ResumeProtected)
	}

	dash := w.runtimeDashboardSnapshot(runtimeDashboardQuery{status: statusDone})
	protected := map[string]bool{}
	for _, run := range dash.Runs {
		protected[run.RunID] = run.ResumeProtected
	}
	if !protected["parent"] {
		t.Fatal("dashboard parent should be resume_protected")
	}
	if protected["unrelated"] {
		t.Fatal("dashboard unrelated run should be deletable")
	}
}

func TestExpireLocalArtifactsKeepsRecentAndLiveLineage(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	old := now.Add(-25 * time.Hour)
	recent := now.Add(-23 * time.Hour)
	parentOld := now.Add(-48 * time.Hour)

	w.mu.Lock()
	w.order = []string{"old", "recent", "parent", "child"}
	w.tiles["old"] = &Tile{RunID: "old", Status: statusDone, Finished: true, Attempts: 2, EndedAt: old, ReplayAvailable: true}
	w.tiles["recent"] = &Tile{RunID: "recent", Status: statusDone, Finished: true, Attempts: 1, EndedAt: recent, ReplayAvailable: true}
	w.tiles["parent"] = &Tile{RunID: "parent", Status: statusDone, Finished: true, Attempts: 1, EndedAt: parentOld, ReplayAvailable: true}
	w.tiles["child"] = &Tile{RunID: "child", Status: statusRunning, Endless: true, ResumeFromRunID: "parent"}
	w.mu.Unlock()

	for _, id := range []string{"old", "recent", "parent"} {
		run, _ := w.snapshotRun(id)
		for _, path := range localFinishDumpPaths(dir, id, max(1, run.Attempts)) {
			writeRetentionFixture(t, path)
		}
		writeRetentionFixture(t, filepath.Join(checkpointAttemptDir(dir, id, 1), "round-001.state"))
	}
	orphanTree := filepath.Join(dir, "checkpoints", "orphan")
	writeRetentionFixture(t, filepath.Join(orphanTree, "1", "round.state"))
	if err := os.Chtimes(orphanTree, old, old); err != nil {
		t.Fatal(err)
	}

	if err := w.expireLocalArtifacts(now, 24*time.Hour); err != nil {
		t.Fatalf("expire: %v", err)
	}

	for _, path := range localFinishDumpPaths(dir, "old", 2) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("old dump still exists %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "checkpoints", safeBase("old"))); !os.IsNotExist(err) {
		t.Fatalf("old checkpoint tree still exists: %v", err)
	}
	oldRun, _ := w.snapshotRun("old")
	if oldRun.ReplayAvailable {
		t.Fatal("expired run still advertises replay")
	}

	for _, id := range []string{"recent", "parent"} {
		if _, err := os.Stat(filepath.Join(dir, safeDumpName(id))); err != nil {
			t.Fatalf("%s dump should survive: %v", id, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "checkpoints", safeBase(id))); err != nil {
			t.Fatalf("%s checkpoint tree should survive: %v", id, err)
		}
	}
	if _, err := os.Stat(orphanTree); !os.IsNotExist(err) {
		t.Fatalf("orphan checkpoint tree still exists: %v", err)
	}
}

func TestExpireLocalArtifactsKeepsPendingFailureDump(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	w.mu.Lock()
	w.tiles["pending"] = &Tile{RunID: "pending", Status: statusDone, Finished: true, Attempts: 1, EndedAt: old}
	w.outbox["pending-attempt-1"] = outboxEntry{ExternalID: "pending-attempt-1", RunID: "pending", Attempt: 1, Status: outboxPending}
	w.mu.Unlock()
	dump := filepath.Join(dir, safeDumpName("pending"))
	writeRetentionFixture(t, dump)
	writeRetentionFixture(t, filepath.Join(checkpointAttemptDir(dir, "pending", 1), "round-001.state"))

	if err := w.expireLocalArtifacts(now, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dump); err != nil {
		t.Fatalf("pending failure dump was removed: %v", err)
	}
}
