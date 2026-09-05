package farm

import "testing"

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
	if len(got) != 1 || got[0] != want[0] {
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
