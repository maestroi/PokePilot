package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveResumeExactCheckpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "round-007-frame-0000012345-travel.state")
	if err := os.WriteFile(path, []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, gotDir, err := resolveResume(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != path || gotDir != dir {
		t.Fatalf("resolveResume = (%q, %q), want (%q, %q)", got, gotDir, path, dir)
	}
}

func TestResolveResumeRunDirectoryChoosesNewestCheckpoint(t *testing.T) {
	runDir := t.TempDir()
	ckptDir := filepath.Join(runDir, "checkpoints")
	if err := os.MkdirAll(ckptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"round-002-frame-0000000200-travel.state",
		"round-010-frame-0000001000-battle.state",
		"round-009-frame-0000009000-heal.state",
		"final.state",
		"round-010-frame-0000001000-battle.knowledge.json",
	} {
		if err := os.WriteFile(filepath.Join(ckptDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, gotDir, err := resolveResume(runDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(ckptDir, "round-010-frame-0000001000-battle.state")
	if got != want || gotDir != ckptDir {
		t.Fatalf("resolveResume = (%q, %q), want (%q, %q)", got, gotDir, want, ckptDir)
	}
}

func TestResolveResumeRejectsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "final.state")
	if err := os.WriteFile(path, []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveResume(path); err == nil {
		t.Fatal("resolveResume(final.state) succeeded, want error")
	}
}

func TestCheckpointRoundFrame(t *testing.T) {
	round, frame, ok := checkpointRoundFrame("round-012-frame-0001234567-go_to.state")
	if !ok || round != 12 || frame != 1234567 {
		t.Fatalf("checkpointRoundFrame = (%d, %d, %t)", round, frame, ok)
	}
	for _, name := range []string{"final.state", "round-x-frame-1-a.state", "round-001-frame-x-a.state", "round-001-frame-0001.state"} {
		if _, _, ok := checkpointRoundFrame(name); ok {
			t.Fatalf("checkpointRoundFrame(%q) unexpectedly accepted", name)
		}
	}
}
