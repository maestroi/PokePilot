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
			Party: []FailurePartyMember{{Species: "Pikachu", Level: 24, HP: 50, MaxHP: 60}},
			Inventory: []FailureInventoryItem{{ID: "poke flute", Quantity: 1}, {ID: "potion", Quantity: 3}},
			Badges:    []string{"Thunder", "Boulder"},
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
		"objective": func(v *FailureIdentity) { v.Objective.Place = "route_13" },
		"cause":     func(v *FailureIdentity) { v.Cause = "no_path" },
		"context":   func(v *FailureIdentity) { v.CauseContext = []string{"can_cut"} },
		"state":     func(v *FailureIdentity) { v.Initial.Party[0].HP-- },
		"outcome":   func(v *FailureIdentity) { v.Outcome = "controller_uncertain" },
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

func TestFailureDetailMarkerRoundTripSurvivesNumberNormalization(t *testing.T) {
	o, err := NewFailureOccurrence(testFailureIdentity(), "build", 4, "round-004.state", "detail", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	marker := FailureDetailMarker(o)
	if marker == "" || !strings.HasPrefix(marker, "failure-id:") {
		t.Fatalf("marker = %q", marker)
	}
	key, fp, ok := ParseFailureDetailMarker(marker)
	if !ok || key != o.Key || fp != o.Fingerprint {
		t.Fatalf("ParseFailureDetailMarker(%q) = %q %q %v, want %q %q true", marker, key, fp, ok, o.Key, o.Fingerprint)
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
