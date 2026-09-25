package farm

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func stateArtifact(name string, data []byte) Artifact {
	sum := sha256.Sum256(data)
	return Artifact{
		Name:      name,
		MediaType: "application/octet-stream",
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}
}

func jsonArtifact(name string, data []byte) Artifact {
	sum := sha256.Sum256(data)
	return Artifact{
		Name:      name,
		MediaType: "application/json",
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}
}

// TestValidateCheckpointStateRejectsEmptyState is the invariant that stopped a
// run's resume lineage from being poisoned: a reader that catches a save file
// between truncate and write observes zero bytes, and a checkpoint published or
// served from that read is one no run can load. The runner then boots a fresh
// cartridge under every retry, forever.
func TestValidateCheckpointStateRejectsEmptyState(t *testing.T) {
	empty := stateArtifact("round-001-frame-0003842410-progress-secret-key-owned.state", nil)
	err := ValidateCheckpointState(empty)
	if err == nil {
		t.Fatal("an empty checkpoint state must be rejected")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("error = %v, want an empty-state cause", err)
	}
}

func TestValidateCheckpointStateAcceptsCompleteState(t *testing.T) {
	full := stateArtifact("round-001-frame-0003842410-progress-secret-key-owned.state", []byte("100kb-of-emulator-registers"))
	if err := ValidateCheckpointState(full); err != nil {
		t.Fatalf("complete state rejected: %v", err)
	}
}

// Knowledge is not a state: a JSON sidecar is validated by the generic artifact
// contract only, so the empty-state rule cannot break paired knowledge checks.
func TestValidateCheckpointStateDoesNotApplyToKnowledge(t *testing.T) {
	knowledge := jsonArtifact(
		"round-001-frame-0003842410-progress-secret-key-owned.knowledge-v6.json",
		[]byte(`{"intent":"take the mansion fall"}`),
	)
	if err := ValidateCheckpointState(knowledge); err != nil {
		t.Fatalf("knowledge sidecar rejected by the state rule: %v", err)
	}
}

// A wall that hands back a remote reference instead of bytes is caught by the
// uploader-side report check, so the runner never writes a zero-byte file for a
// checkpoint whose payload never travelled.
func TestValidateCheckpointReportRejectsEmptyState(t *testing.T) {
	report := CheckpointReport{
		RunID:     "run-42",
		Attempt:   7,
		Artifacts: []Artifact{stateArtifact("round-003-frame-0000000300-go-to-route-9.state", nil)},
	}
	err := ValidateCheckpointReport(report)
	if err == nil {
		t.Fatal("a checkpoint report carrying an empty state must be rejected")
	}
}

func TestValidateCheckpointReportAcceptsCompletePair(t *testing.T) {
	report := CheckpointReport{
		RunID:   "run-42",
		Attempt: 7,
		Artifacts: []Artifact{
			stateArtifact("round-003-frame-0000000300-go-to-route-9.state", []byte("complete-state")),
			jsonArtifact("round-003-frame-0000000300-go-to-route-9.knowledge-v6.json", []byte(`{"intent":"leave pewter"}`)),
		},
	}
	if err := ValidateCheckpointReport(report); err != nil {
		t.Fatalf("complete checkpoint pair rejected: %v", err)
	}
}
