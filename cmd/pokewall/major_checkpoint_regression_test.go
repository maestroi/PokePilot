package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLatestMajorResumeCheckpointPrefersHighestBadgeAcrossAttempts(t *testing.T) {
	dumps := t.TempDir()
	writeMajorPairToAttempt(t, dumps, "campaign", 1, 3, 30, "badge-three")
	writeMajorPairToAttempt(t, dumps, "campaign", 2, 2, 40, "newer-but-older-badge")

	cp, err := latestMajorResumeCheckpoint(dumps, "campaign", 2)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Attempt != 1 {
		t.Fatalf("resume attempt = %d, want 1 containing the higher badge", cp.Attempt)
	}
	badge, ok := majorCheckpointBadge(cp.State.Name)
	if !ok || badge != 3 {
		t.Fatalf("resume checkpoint = %q, want badge 3", cp.State.Name)
	}
}

func TestLatestLineageMajorCheckpointPrefersHighestBadgeAcrossParents(t *testing.T) {
	dumps := t.TempDir()
	w := NewWall(dumps)
	w.mu.Lock()
	w.tiles["parent"] = &Tile{RunID: "parent", Attempts: 1}
	w.tiles["child"] = &Tile{RunID: "child", Attempts: 1, ResumeFromRunID: "parent"}
	w.mu.Unlock()

	writeMajorPairToAttempt(t, dumps, "parent", 1, 3, 30, "badge-three")
	writeMajorPairToAttempt(t, dumps, "child", 1, 2, 40, "child-badge-two")

	cp, err := w.latestLineageMajorCheckpoint("child")
	if err != nil {
		t.Fatal(err)
	}
	badge, ok := majorCheckpointBadge(cp.State.Name)
	if !ok || badge != 3 {
		t.Fatalf("lineage resume checkpoint = %q, want highest badge 3", cp.State.Name)
	}
	if string(cp.State.Data) != "badge-three" {
		t.Fatalf("lineage resume state = %q, want parent badge-three state", cp.State.Data)
	}
}

func writeMajorPairToAttempt(t *testing.T, dumps, runID string, attempt, badge, round int, stateData string) {
	t.Helper()
	dir := checkpointAttemptDir(dumps, runID, attempt)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	state, knowledge := majorWallPair(badge, round, stateData)
	if err := os.WriteFile(filepath.Join(dir, state.Name), state.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, knowledge.Name), knowledge.Data, 0o644); err != nil {
		t.Fatal(err)
	}
}
