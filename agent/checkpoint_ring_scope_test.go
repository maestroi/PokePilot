package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCheckpointRingFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func requireCheckpointFile(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if want && err != nil {
		t.Fatalf("expected %s to survive: %v", path, err)
	}
	if !want && !os.IsNotExist(err) {
		t.Fatalf("expected %s to be evicted, stat err=%v", path, err)
	}
}

func TestCheckpointRingEvictsOnlyRoundCheckpoints(t *testing.T) {
	dir := t.TempDir()
	oldRound := filepath.Join(dir, "round-001-frame-0000000001-old.state")
	newRound := filepath.Join(dir, "round-002-frame-0000000002-new.state")
	periodic := filepath.Join(dir, "periodic-0000000003.state")
	periodicMeta := filepath.Join(dir, "periodic-0000000003.json")
	major := filepath.Join(dir, "major-badge-1-round-002-frame-0000000002-test.state")
	majorKnowledge := knowledgePathForState(major)
	orphanRoundKnowledge := knowledgePathForState(filepath.Join(dir, "round-000-frame-0000000000-orphan.state"))

	for _, path := range []string{
		oldRound, knowledgePathForState(oldRound),
		newRound, knowledgePathForState(newRound),
		periodic, periodicMeta,
		major, majorKnowledge,
		orphanRoundKnowledge,
	} {
		writeCheckpointRingFixture(t, path)
	}

	ring := checkpointRing{dir: dir, keep: 1}
	if err := ring.evict(); err != nil {
		t.Fatalf("evict: %v", err)
	}

	requireCheckpointFile(t, oldRound, false)
	requireCheckpointFile(t, knowledgePathForState(oldRound), false)
	requireCheckpointFile(t, newRound, true)
	requireCheckpointFile(t, knowledgePathForState(newRound), true)
	requireCheckpointFile(t, orphanRoundKnowledge, false)
	requireCheckpointFile(t, periodic, true)
	requireCheckpointFile(t, periodicMeta, true)
	requireCheckpointFile(t, major, true)
	requireCheckpointFile(t, majorKnowledge, true)
}
