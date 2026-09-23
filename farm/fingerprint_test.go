package farm

import (
	"strings"
	"testing"
	"time"
)

func testFailureIdentity() FailureIdentity {
	return FailureIdentity{
		Version: FailureIdentityVersion,
		Game:    "pokemon",
		Adapter: "pokemon-red",
		Objective: FailureObjective{
			Kind:  "go_to",
			Place: "route 12 south of snorlax",
			Flee:  true,
		},
		Outcome:      "blocked",
		Cause:        "route_prerequisite_missing",
		CauseContext: []string{"can_clear_snorlax", "can_surf"},
		Initial: FailureState{
			Location:     "route_12",
			X:            10,
			Y:            61,
			Controllable: true,
			Money:        1234,
			Party:        []FailurePartyMember{{Species: "Pikachu", Level: 24, HP: 50, MaxHP: 60}},
			Inventory:    []FailureInventoryItem{{ID: "poke flute", Quantity: 1}, {ID: "potion", Quantity: 3}},
			Badges:       []string{"Thunder", "Boulder"},
			Capabilities: []FailureCapability{
				{ID: "can_surf", BadgeOwned: true},
				{ID: "can_cut", BadgeOwned: true, Learned: true, Usable: true},
			},
			Progress: []FailureProgressFact{{ID: "poke_flute_acquired", Complete: true}},
		},
		Final: FailureState{
			Location:     "route_12",
			X:            10,
			Y:            61,
			Controllable: true,
		},
	}
}

func TestFailureFingerprintCanonicalSets(t *testing.T) {
	a := testFailureIdentity()
	b := testFailureIdentity()
	b.Initial.Inventory[0], b.Initial.Inventory[1] = b.Initial.Inventory[1], b.Initial.Inventory[0]
	b.Initial.Badges[0], b.Initial.Badges[1] = b.Initial.Badges[1], b.Initial.Badges[0]
	b.Initial.Capabilities[0], b.Initial.Capabilities[1] = b.Initial.Capabilities[1], b.Initial.Capabilities[0]
	b.CauseContext[0], b.CauseContext[1] = b.CauseContext[1], b.CauseContext[0]

	ka, fa, err := FingerprintFailureIdentity(a)
	if err != nil {
		t.Fatal(err)
	}
	kb, fb, err := FingerprintFailureIdentity(b)
	if err != nil {
		t.Fatal(err)
	}
	if ka != kb || fa != fb {
		t.Fatalf("canonical reordering changed fingerprint: %s/%s != %s/%s", ka, fa, kb, fb)
	}
}

func TestFailureFingerprintIgnoresOccurrenceMetadata(t *testing.T) {
	id := testFailureIdentity()
	a, err := NewFailureOccurrence(id, "build-a", 7, "round-007.state", "first diagnostic prose", time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewFailureOccurrence(id, "build-b", 99, "other.state", "completely different diagnostic prose", time.Unix(20, 0))
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint != b.Fingerprint || a.Key != b.Key {
		t.Fatalf("build/checkpoint/diagnostic changed logical fingerprint: %+v vs %+v", a, b)
	}
}

func TestFailureFingerprintChangesForMaterialIdentity(t *testing.T) {
	base := testFailureIdentity()
	_, want, err := FingerprintFailureIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*FailureIdentity){
		"objective":        func(v *FailureIdentity) { v.Objective.Place = "route_13" },
		"field capability": func(v *FailureIdentity) { v.Objective.FieldCapability = "surf" },
		"cause":            func(v *FailureIdentity) { v.Cause = "no_path" },
		"context":          func(v *FailureIdentity) { v.CauseContext = []string{"can_cut"} },
		"state":            func(v *FailureIdentity) { v.Initial.Party[0].HP-- },
		"outcome":          func(v *FailureIdentity) { v.Outcome = "controller_uncertain" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			v := testFailureIdentity()
			mutate(&v)
			_, got, err := FingerprintFailureIdentity(v)
			if err != nil {
				t.Fatal(err)
			}
			if got == want {
				t.Fatalf("material %s change did not change fingerprint %s", name, got)
			}
		})
	}
}

func TestFailureFamilyFingerprintIgnoresVolatileReplayState(t *testing.T) {
	a := testFailureIdentity()
	b := testFailureIdentity()
	b.Initial.X++
	b.Initial.Y++
	b.Initial.Money += 500
	b.Initial.Party[0].HP--
	b.Initial.Party[0].Level++
	b.Initial.Inventory[0].Quantity++
	b.Final.X++
	b.Final.Y++

	ka, fa, err := FingerprintFailureFamily(a)
	if err != nil {
		t.Fatal(err)
	}
	kb, fb, err := FingerprintFailureFamily(b)
	if err != nil {
		t.Fatal(err)
	}
	if ka != kb || fa != fb {
		t.Fatalf("volatile replay state split failure family: %s/%s != %s/%s", ka, fa, kb, fb)
	}
	_, exactA, _ := FingerprintFailureIdentity(a)
	_, exactB, _ := FingerprintFailureIdentity(b)
	if exactA == exactB {
		t.Fatal("exact occurrence fingerprint unexpectedly ignored replay state")
	}
}

func TestFailureFamilyFingerprintCollapsesCrossMapSemanticCause(t *testing.T) {
	a := testFailureIdentity()
	a.Objective = FailureObjective{Kind: "progress", Progress: "fly_ready"}
	a.Outcome = "blocked"
	a.Cause = "field_roster_no_recovery"
	a.CauseContext = nil
	a.Initial.Location, a.Final.Location = "celadon city", "celadon city"

	b := a
	b.Initial.Location, b.Final.Location = "fuchsia city", "fuchsia city"

	_, fa, err := FingerprintFailureFamily(a)
	if err != nil {
		t.Fatal(err)
	}
	_, fb, err := FingerprintFailureFamily(b)
	if err != nil {
		t.Fatal(err)
	}
	if fa != fb {
		t.Fatalf("semantic field-roster family split by location: %s != %s", fa, fb)
	}
}

func TestFailureFamilyFingerprintKeepsBroadLocalCausesSeparate(t *testing.T) {
	a := testFailureIdentity()
	a.Objective = FailureObjective{Kind: "progress", Progress: "secret_key_owned"}
	a.Outcome = "blocked"
	a.Cause = "dialogue_interrupted"
	a.CauseContext = nil
	a.Initial.Location, a.Final.Location = "cinnabar island", "cinnabar island"

	b := a
	b.Initial.Location, b.Final.Location = "pokemon mansion b1f", "pokemon mansion b1f"

	_, fa, err := FingerprintFailureFamily(a)
	if err != nil {
		t.Fatal(err)
	}
	_, fb, err := FingerprintFailureFamily(b)
	if err != nil {
		t.Fatal(err)
	}
	if fa == fb {
		t.Fatalf("location-sensitive dialogue failures collapsed to %s", fa)
	}
}

func TestFailureDetailMarkerRoundTripSurvivesNumberNormalization(t *testing.T) {
	id := testFailureIdentity()
	id.Objective.Progress = "fuchsia_progress"
	id.Objective.FieldCapability = "surf"
	o, err := NewFailureOccurrence(id, "build", 4, "round-004.state", "detail", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	marker := FailureDetailMarker(o)
	if marker == "" || !strings.HasPrefix(marker, "failure-id:") {
		t.Fatalf("marker = %q", marker)
	}
	for _, want := range []string{
		"progress=fuchsia_progress",
		"field_capability=surf",
		"cause_context=can_clear_snorlax,can_surf",
	} {
		if !strings.Contains(marker, want) {
			t.Fatalf("marker %q missing %q", marker, want)
		}
	}
	if got := strings.Join(ParseFailureDetailCauseContext(marker), ","); got != "can_clear_snorlax,can_surf" {
		t.Fatalf("ParseFailureDetailCauseContext(%q) = %q", marker, got)
	}
	key, fp, ok := ParseFailureDetailMarker(marker)
	if !ok || key != o.Key || fp != o.Fingerprint {
		t.Fatalf("ParseFailureDetailMarker(%q) = %q %q %v, want %q %q true", marker, key, fp, ok, o.Key, o.Fingerprint)
	}
	familyKey, familyFP, err := FingerprintFailureFamily(o.Identity)
	if err != nil {
		t.Fatal(err)
	}
	gotFamilyKey, gotFamilyFP, ok := ParseFailureDetailFamilyMarker(marker)
	if !ok || gotFamilyKey != familyKey || gotFamilyFP != familyFP {
		t.Fatalf("ParseFailureDetailFamilyMarker(%q) = %q %q %v, want %q %q true", marker, gotFamilyKey, gotFamilyFP, ok, familyKey, familyFP)
	}
}

func TestValidateFailureOccurrenceRejectsTampering(t *testing.T) {
	o, err := NewFailureOccurrence(testFailureIdentity(), "build", 1, "", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	o.Identity.Cause = "different"
	if err := ValidateFailureOccurrence(o); err == nil {
		t.Fatal("tampered identity accepted")
	}
}
