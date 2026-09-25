package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func priorityTestArtifact(name string, size int) farm.Artifact {
	data := []byte(strings.Repeat("x", size))
	sum := sha256.Sum256(data)
	return farm.Artifact{
		Name:      name,
		MediaType: "application/octet-stream",
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}
}

func TestAppendPriorityFinishArtifactEvictsBulkyCheckpointEvidence(t *testing.T) {
	runContext := priorityTestArtifact(farm.RunContextArtifactName, 128)
	large := priorityTestArtifact("round-010-checkpoint.state", 13<<20)
	smaller := priorityTestArtifact("round-009-checkpoint.state", (11<<20)-512)
	priority := priorityTestArtifact(farm.ObjectiveFailureArtifactName, 2048)

	report := farm.FinishReport{Artifacts: []farm.Artifact{runContext, large, smaller}}
	if err := farm.ValidateFinishArtifacts(report); err != nil {
		t.Fatalf("fixture exceeds budget before priority artifact: %v", err)
	}

	evicted, err := appendPriorityFinishArtifact(&report, priority)
	if err != nil {
		t.Fatalf("appendPriorityFinishArtifact: %v", err)
	}
	if len(evicted) != 1 || evicted[0] != large.Name {
		t.Fatalf("evicted = %v, want [%s]", evicted, large.Name)
	}
	if err := farm.ValidateFinishArtifacts(report); err != nil {
		t.Fatalf("result invalid: %v", err)
	}

	names := map[string]bool{}
	for _, artifact := range report.Artifacts {
		names[artifact.Name] = true
	}
	for _, want := range []string{farm.RunContextArtifactName, smaller.Name, farm.ObjectiveFailureArtifactName} {
		if !names[want] {
			t.Fatalf("missing preserved artifact %q from %v", want, names)
		}
	}
	if names[large.Name] {
		t.Fatalf("largest lower-priority artifact %q was not evicted", large.Name)
	}
}

func TestAppendPriorityFinishArtifactKeepsExistingEvidenceWhenItFits(t *testing.T) {
	existing := priorityTestArtifact("round-001-checkpoint.state", 1024)
	priority := priorityTestArtifact(farm.ObjectiveFailureArtifactName, 1024)
	report := farm.FinishReport{Artifacts: []farm.Artifact{existing}}

	evicted, err := appendPriorityFinishArtifact(&report, priority)
	if err != nil {
		t.Fatal(err)
	}
	if len(evicted) != 0 {
		t.Fatalf("evicted = %v, want none", evicted)
	}
	if len(report.Artifacts) != 2 || report.Artifacts[0].Name != existing.Name || report.Artifacts[1].Name != priority.Name {
		t.Fatalf("artifacts = %+v", report.Artifacts)
	}
}
