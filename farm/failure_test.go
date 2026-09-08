package farm

import (
	"reflect"
	"testing"
)

func TestObjectiveFailureArtifactRoundTrip(t *testing.T) {
	want := []ObjectiveFailure{{
		Objective:  "go to mt moon b1f, fleeing wild battles",
		Error:      "skill: step left blocked at (10,22)",
		Count:      2,
		FirstRound: 14,
		LastRound:  15,
		Map:        0x3b,
		X:          10,
		Y:          22,
		Blocking:   true,
	}}
	artifact, err := NewObjectiveFailureArtifact(want)
	if err != nil {
		t.Fatalf("NewObjectiveFailureArtifact: %v", err)
	}
	if artifact.Name != ObjectiveFailureArtifactName || artifact.MediaType != "application/json" || artifact.SHA256 == "" {
		t.Fatalf("artifact = %+v", artifact)
	}
	got, err := DecodeObjectiveFailures(FinishReport{Artifacts: []Artifact{artifact}})
	if err != nil {
		t.Fatalf("DecodeObjectiveFailures: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded = %+v, want %+v", got, want)
	}
}

func TestObjectiveFailureArtifactMissingIsBackwardCompatible(t *testing.T) {
	artifact, err := NewObjectiveFailureArtifact(nil)
	if err != nil {
		t.Fatalf("NewObjectiveFailureArtifact(nil): %v", err)
	}
	if artifact.Name != "" {
		t.Fatalf("empty telemetry produced artifact %+v", artifact)
	}
	got, err := DecodeObjectiveFailures(FinishReport{})
	if err != nil {
		t.Fatalf("DecodeObjectiveFailures(old report): %v", err)
	}
	if got != nil {
		t.Fatalf("old report decoded %+v, want nil", got)
	}
}

func TestObjectiveFailureArtifactPreservesImpactCounts(t *testing.T) {
	want := []ObjectiveFailure{{Objective: "buy 3 pokeball", Error: "shop menu timeout", Count: 4, RecoveredCount: 3, TerminalCount: 1}}
	artifact, err := NewObjectiveFailureArtifact(want)
	if err != nil {
		t.Fatalf("artifact: %v", err)
	}
	got, err := DecodeObjectiveFailures(FinishReport{Artifacts: []Artifact{artifact}})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestObjectiveFailureRejectsImpossibleImpactCounts(t *testing.T) {
	_, err := NewObjectiveFailureArtifact([]ObjectiveFailure{{Objective: "x", Count: 1, RecoveredCount: 1, TerminalCount: 1}})
	if err == nil {
		t.Fatal("accepted recovered+terminal counts greater than occurrence count")
	}
}
