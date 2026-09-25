package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func writeProfileIdentityFixture(t *testing.T, gameID game.GameID, revision game.RevisionID) string {
	t.Helper()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-001-frame-0000000001-test.state")
	if err := os.WriteFile(statePath, []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeMemoryFileForProfile(statePath, NewKnowledge(nil), gameID, revision, "", 0); err != nil {
		t.Fatal(err)
	}
	return statePath
}

func TestValidateCheckpointProfileRejectsDifferentGame(t *testing.T) {
	statePath := writeProfileIdentityFixture(t, "pokemon-yellow", "en-us-rev0")
	err := ValidateCheckpointProfile(statePath, "pokemon-red", "en-us-rev0")
	if err == nil || !strings.Contains(err.Error(), "pokemon-yellow") || !strings.Contains(err.Error(), "pokemon-red") {
		t.Fatalf("ValidateCheckpointProfile mismatch = %v", err)
	}
}

func TestValidateCheckpointProfileRejectsDifferentRevision(t *testing.T) {
	statePath := writeProfileIdentityFixture(t, "pokemon-yellow", "en-us-rev0")
	err := ValidateCheckpointProfile(statePath, "pokemon-yellow", "en-us-rev1")
	if err == nil || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("ValidateCheckpointProfile revision mismatch = %v", err)
	}
}

func TestValidateCheckpointProfileAcceptsMatchingIdentity(t *testing.T) {
	statePath := writeProfileIdentityFixture(t, "pokemon-yellow", "en-us-rev0")
	if err := ValidateCheckpointProfile(statePath, "pokemon-yellow", "en-us-rev0"); err != nil {
		t.Fatalf("matching identity: %v", err)
	}
}

func TestValidateCheckpointProfileAllowsLegacyIdentitylessCheckpoint(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-001-frame-0000000001-old.state")
	if err := os.WriteFile(statePath, []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeMemoryFile(statePath, NewKnowledge(nil), "", 0); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCheckpointProfile(statePath, "pokemon-red", "en-us-rev0"); err != nil {
		t.Fatalf("legacy checkpoint should remain readable: %v", err)
	}
}
