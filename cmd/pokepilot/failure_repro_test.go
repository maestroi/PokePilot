package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestAppendFailureReproArtifacts(t *testing.T) {
	identity := farm.FailureIdentity{
		Version: farm.FailureIdentityVersion,
		Game:    "pokemon",
		Adapter: "pokemon-red",
		Objective: farm.FailureObjective{
			Kind:  "go_to",
			Place: "route 9",
		},
		Outcome: "blocked",
		Cause:   "navigation_stalled",
		Initial: farm.FailureState{Location: "cerulean city", X: 10, Y: 6, Controllable: true},
		Final:   farm.FailureState{Location: "route 9", X: 4, Y: 12, Controllable: true},
	}
	key, fingerprint, err := farm.FingerprintFailureIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	checkpointName := "round-012-frame-0000012345-go-to-route-9.state"
	checkpointData := []byte("checked-emulator-state")
	report := farm.FinishReport{
		RunID:         "run-fixer",
		Attempt:       3,
		RunnerVersion: "cafebabe",
		Artifacts: []farm.Artifact{
			testReproArtifact(checkpointName, "application/octet-stream", checkpointData),
			testReproArtifact(strings.TrimSuffix(checkpointName, ".state")+".knowledge-v4.json", "application/json", []byte(`{"intent":"continue"}`)),
		},
	}
	failure := farm.ObjectiveFailure{
		Objective:   "go to route 9",
		Error:       "navigation stalled",
		Count:       1,
		FirstRound:  12,
		LastRound:   12,
		Key:         key,
		Fingerprint: fingerprint,
		Identity:    &identity,
		Build:       "cafebabe",
		Outcome:     identity.Outcome,
		Cause:       identity.Cause,
		Checkpoint:  checkpointName,
	}

	appendFailureReproArtifacts(&report, []farm.ObjectiveFailure{failure})
	if len(report.Artifacts) != 3 {
		t.Fatalf("artifacts = %d, want checkpoint pair + repro", len(report.Artifacts))
	}
	repro := report.Artifacts[2]
	if !strings.HasPrefix(repro.Name, "round-012-") || !strings.HasSuffix(repro.Name, "."+farm.FailureReproArtifactName) {
		t.Fatalf("repro name = %q", repro.Name)
	}
	bundle, err := farm.DecodeFailureRepro(repro.Data)
	if err != nil {
		t.Fatalf("DecodeFailureRepro: %v", err)
	}
	if bundle.RunID != report.RunID || bundle.Attempt != 3 || bundle.Fingerprint != fingerprint {
		t.Fatalf("bundle = %+v", bundle)
	}
	if bundle.Checkpoint.Name != checkpointName || !strings.Contains(bundle.SuggestedCommand, "cmd/pokerepro") || !strings.Contains(bundle.SuggestedCommand, checkpointName) {
		t.Fatalf("repro handoff = %+v", bundle)
	}
}

func testReproArtifact(name, mediaType string, data []byte) farm.Artifact {
	sum := sha256.Sum256(data)
	return farm.Artifact{
		Name:      name,
		MediaType: mediaType,
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}
}
