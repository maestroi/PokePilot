package farm

import (
	"strings"
	"testing"
)

func TestFailureReproArtifactRoundTrip(t *testing.T) {
	identity := FailureIdentity{
		Version: FailureIdentityVersion,
		Game:    "pokemon",
		Adapter: "pokemon-red",
		Objective: FailureObjective{
			Kind:  "go_to",
			Place: "route 9",
			Flee:  true,
		},
		Outcome: "blocked",
		Cause:   "navigation_stalled",
		Initial: FailureState{Location: "cerulean city", X: 20, Y: 8, Controllable: true},
		Final:   FailureState{Location: "route 9", X: 4, Y: 12, Controllable: true},
	}
	key, fp, err := FingerprintFailureIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	failure := ObjectiveFailure{
		Objective:   "go to route 9, fleeing wild battles",
		Error:       "navigation stalled",
		Count:       1,
		FirstRound:  7,
		LastRound:   7,
		Key:         key,
		Fingerprint: fp,
		Identity:    &identity,
		Build:       "deadbeef",
		Outcome:     identity.Outcome,
		Cause:       identity.Cause,
		Checkpoint:  "round-007-frame-0000012345-go-to-route-9.state",
	}
	report := FinishReport{
		RunID:         "run-repro",
		Attempt:       2,
		RunnerVersion: "fallback-build",
		Artifacts: []Artifact{{
			Name:      failure.Checkpoint,
			MediaType: "application/octet-stream",
			SHA256:    strings.Repeat("a", 64),
			Data:      []byte("checked-state"),
		}},
	}

	artifact, err := NewFailureReproArtifact(report, failure)
	if err != nil {
		t.Fatalf("NewFailureReproArtifact: %v", err)
	}
	if artifact.Name != "round-007-frame-0000012345-go-to-route-9."+FailureReproArtifactName || len(artifact.Data) == 0 {
		t.Fatalf("artifact = %+v", artifact)
	}
	bundle, err := DecodeFailureRepro(artifact.Data)
	if err != nil {
		t.Fatalf("DecodeFailureRepro: %v", err)
	}
	if bundle.RunID != report.RunID || bundle.Attempt != 2 || bundle.ObservedRevision != "deadbeef" {
		t.Fatalf("bundle occurrence = %+v", bundle)
	}
	if bundle.Fingerprint != fp || bundle.Checkpoint.Name != failure.Checkpoint || bundle.Checkpoint.SHA256 != strings.Repeat("a", 64) {
		t.Fatalf("bundle identity/checkpoint = %+v", bundle)
	}
	wantCommand := `go run ./cmd/pokerepro -run "run-repro" -attempt 2 -checkpoint "round-007-frame-0000012345-go-to-route-9.state"`
	if bundle.SuggestedCommand != wantCommand {
		t.Fatalf("command = %q, want %q", bundle.SuggestedCommand, wantCommand)
	}
}

func TestFailureReproRequiresStructuredCheckpoint(t *testing.T) {
	for _, failure := range []ObjectiveFailure{
		{Checkpoint: "round-001.state"},
		{Identity: &FailureIdentity{}},
	} {
		artifact, err := NewFailureReproArtifact(FinishReport{}, failure)
		if err != nil {
			t.Fatalf("NewFailureReproArtifact: %v", err)
		}
		if artifact.Name != "" {
			t.Fatalf("unexpected artifact = %+v", artifact)
		}
	}
}
