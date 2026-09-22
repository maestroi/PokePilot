package main

import (
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func terminalMarkerOccurrence(t *testing.T) farm.FailureOccurrence {
	t.Helper()
	identity := farm.FailureIdentity{
		Version: farm.FailureIdentityVersion,
		Game:    "pokemon",
		Adapter: "pokemon-red",
		Objective: farm.FailureObjective{
			Kind:  "go_to",
			Place: "fuchsia city",
		},
		Outcome:      "blocked",
		Cause:        "route_prerequisite_missing",
		CauseContext: []string{"can_clear_snorlax"},
	}
	occurrence, err := farm.NewFailureOccurrence(identity, "build-a", 7, "", "route blocked", time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	return occurrence
}

func TestObjectiveFailureFingerprintRecoversEmbeddedTerminalMarker(t *testing.T) {
	occurrence := terminalMarkerOccurrence(t)
	marker := farm.FailureDetailMarker(occurrence)
	failure := farm.ObjectiveFailure{
		Objective: "recover from repeated objective failures",
		Error:     "failure recovery budget was exhausted: " + marker,
		Cause:     "failure-budget",
	}

	key, fingerprint, structured, err := objectiveFailureFingerprint(failure)
	if err != nil {
		t.Fatal(err)
	}
	if !structured {
		t.Fatal("embedded canonical marker was treated as legacy prose")
	}
	if key != occurrence.Key || fingerprint != occurrence.Fingerprint {
		t.Fatalf("fingerprint = %q %q, want %q %q", key, fingerprint, occurrence.Key, occurrence.Fingerprint)
	}
}

func TestEmbeddedFailureMarkerRejectsIncidentalText(t *testing.T) {
	for _, text := range []string{
		"failure recovery budget was exhausted",
		"failure-id:not-a-valid-digest",
		"prefix failure-id:1234 suffix",
	} {
		if key, fingerprint, ok := embeddedFailureMarker(text); ok {
			t.Fatalf("embeddedFailureMarker(%q) = %q %q true", text, key, fingerprint)
		}
	}
}

func TestTerminalRunFailureMarkerUsesCanonicalFingerprint(t *testing.T) {
	occurrence := terminalMarkerOccurrence(t)
	failure, ok := terminalRunFailure(farm.FinishReport{
		Reason: "failed",
		Detail: farm.FailureDetailMarker(occurrence),
	}, nil)
	if !ok {
		t.Fatal("terminalRunFailure did not synthesize fallback")
	}
	if !strings.Contains(failure.Error, "failure-id:") {
		t.Fatalf("synthetic error lost marker: %q", failure.Error)
	}
	if got := strings.Join(failure.CauseContext, ","); got != "can_clear_snorlax" {
		t.Fatalf("synthetic cause context = %q, want can_clear_snorlax", got)
	}
	key, fingerprint, structured, err := objectiveFailureFingerprint(failure)
	if err != nil {
		t.Fatal(err)
	}
	if !structured || key != occurrence.Key || fingerprint != occurrence.Fingerprint {
		t.Fatalf("synthetic fingerprint = %q %q structured=%v, want %q %q true", key, fingerprint, structured, occurrence.Key, occurrence.Fingerprint)
	}
}
