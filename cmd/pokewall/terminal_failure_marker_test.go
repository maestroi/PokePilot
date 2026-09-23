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
	wantKey, wantFingerprint, err := farm.FingerprintFailureFamily(occurrence.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if key != wantKey || fingerprint != wantFingerprint {
		t.Fatalf("family fingerprint = %q %q, want %q %q", key, fingerprint, wantKey, wantFingerprint)
	}
	exactKey, exactFingerprint, _, err := objectiveFailureOccurrenceFingerprint(failure)
	if err != nil {
		t.Fatal(err)
	}
	if exactKey != occurrence.Key || exactFingerprint != occurrence.Fingerprint {
		t.Fatalf("occurrence fingerprint = %q %q, want %q %q", exactKey, exactFingerprint, occurrence.Key, occurrence.Fingerprint)
	}
}

func TestEmbeddedTerminalMarkersAggregateVolatileOccurrencesByFamily(t *testing.T) {
	base := terminalMarkerOccurrence(t)
	changedIdentity := base.Identity
	changedIdentity.Initial = farm.FailureState{
		Location: "route 12",
		X:        10,
		Y:        20,
		Money:    500,
		Party:    []farm.FailurePartyMember{{Species: "pikachu", Level: 30, HP: 12, MaxHP: 80}},
	}
	changedIdentity.Final = farm.FailureState{
		Location: "route 12",
		X:        11,
		Y:        20,
		Money:    300,
		Party:    []farm.FailurePartyMember{{Species: "pikachu", Level: 30, HP: 8, MaxHP: 80}},
	}
	changed, err := farm.NewFailureOccurrence(changedIdentity, "build-b", 8, "", "same semantic blocker", time.Unix(200, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if base.Fingerprint == changed.Fingerprint {
		t.Fatal("test setup did not produce distinct exact occurrence fingerprints")
	}

	wrap := func(o farm.FailureOccurrence) farm.ObjectiveFailure {
		return farm.ObjectiveFailure{
			Objective: "recover from repeated objective failures",
			Error:     "failure recovery budget was exhausted: " + farm.FailureDetailMarker(o),
			Cause:     "failure-budget",
		}
	}
	keyA, fpA, _, err := objectiveFailureFingerprint(wrap(base))
	if err != nil {
		t.Fatal(err)
	}
	keyB, fpB, _, err := objectiveFailureFingerprint(wrap(changed))
	if err != nil {
		t.Fatal(err)
	}
	if keyA != keyB || fpA != fpB {
		t.Fatalf("volatile terminal occurrences split issue family: %s/%s != %s/%s", keyA, fpA, keyB, fpB)
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
	wantKey, wantFingerprint, err := farm.FingerprintFailureFamily(occurrence.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if !structured || key != wantKey || fingerprint != wantFingerprint {
		t.Fatalf("synthetic family fingerprint = %q %q structured=%v, want %q %q true", key, fingerprint, structured, wantKey, wantFingerprint)
	}
}
