package agent

import "testing"

func TestTravelTrainerBlackoutFingerprintChangesAfterPartyProgress(t *testing.T) {
	obj := Objective{Kind: KindGoTo, Place: "pewter gym", Flee: true}
	before := Observation{
		Location:     "viridian pokemon center",
		X:            3,
		Y:            3,
		Controllable: true,
		Party: []PartyMon{{
			Species: "charmander",
			Level:   8,
			HP:      24,
			MaxHP:   24,
		}},
		LeadPP: []uint8{25, 15},
	}
	afterTraining := before
	afterTraining.Party = []PartyMon{{
		Species: "charmander",
		Level:   10,
		HP:      29,
		MaxHP:   29,
	}}

	first := ObjectiveResult{Objective: obj, Outcome: OutcomeBlocked, Cause: "trainer_blacked_out", Final: before}
	second := ObjectiveResult{Objective: obj, Outcome: OutcomeBlocked, Cause: "trainer_blacked_out", Final: afterTraining}
	if recoverableFailureKey(obj, first) == recoverableFailureKey(obj, second) {
		t.Fatal("trainer blackout fingerprint ignored material party progress during travel")
	}
}

func TestTravelNavigationFingerprintStillIgnoresPartyProgress(t *testing.T) {
	obj := Objective{Kind: KindGoTo, Place: "pewter gym", Flee: true}
	before := Observation{
		Location:     "route 2",
		X:            8,
		Y:            71,
		Controllable: true,
		Party: []PartyMon{{
			Species: "charmander",
			Level:   8,
			HP:      12,
			MaxHP:   24,
		}},
	}
	afterTraining := before
	afterTraining.Party = []PartyMon{{
		Species: "charmander",
		Level:   10,
		HP:      29,
		MaxHP:   29,
	}}

	first := ObjectiveResult{Objective: obj, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: before}
	second := ObjectiveResult{Objective: obj, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: afterTraining}
	if recoverableFailureKey(obj, first) != recoverableFailureKey(obj, second) {
		t.Fatal("ordinary travel navigation fingerprint became combat-sensitive")
	}
}
