package agent

import "testing"

// TestOfferWithholdsUnroutableHealDestination: a heal objective that names a
// Pokemon Center is a journey, and the live router's positive rejection of that
// destination must withhold it exactly like an unroutable "go to" journey.
//
// MEASURED 2026-09-23 on run-jxh8lk19wv6on. The run was locked inside the
// Elite Four at LORELEIS_ROOM (4,9) with a hurt party and no healing items, so
// the recovery provider's sentence was legitimate — but the only Center it
// offered, "heal the party at INDIGO PLATEAU POKEMON CENTER", sits in the
// lobby BEHIND the player. Pokemon Red refuses that walk on purpose
// (pokered/scripts/LoreleisRoom.asm: standing on the room's entrance tiles
// displays "Someone's voice: Don't run away!" and pushes the player back up),
// so the objective could never complete. The run re-picked it for 80 matching
// failures and surfaced as unknown_failure/unknown_error.
func TestOfferWithholdsUnroutableHealDestination(t *testing.T) {
	known := testKnowledge(map[uint8][]uint8{0xAE: {0xF5}, 0xF5: {0xAE}})
	known.SawMap(0xAE)
	known.SawMap(0xF5)

	obs := Observation{
		GameID:     testGameID,
		Map:        0xF5,
		MapName:    "LORELEIS_ROOM",
		X:          4,
		Y:          9,
		PartyCount: 1,
		Party:      []PartyMon{{Species: "venusaur", Level: 78, HP: 68, MaxHP: 249}},
	}

	if place := PlaceID("indigo plateau pokemon center"); !offersPlace(Offer(obs, known), string(place)) {
		t.Fatalf("test assumption wrong: %s is not on the menu even before the router is consulted", place)
	}

	// The live router says the lobby is behind a closed league gate.
	obs.Unroutable = []string{"indigo plateau pokemon center"}
	offer := OfferWithEvidence(obs, known)
	for _, o := range offer.Candidates {
		if o.Kind == KindHeal {
			t.Fatalf("heal at an unroutable Center stayed on the menu: %s", o)
		}
	}

	// Withholding is not enough on its own: the planner has to be told which
	// destination the router rejected, or it can only guess why recovery vanished.
	found := false
	for _, b := range offer.Blocked {
		if b.Place == PlaceID("indigo plateau pokemon center") && b.Reason == "route_unroutable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("withheld Center produced no route_unroutable evidence: %+v", offer.Blocked)
	}
}

// TestRecoveryFallsBackToRoutableCenter: the active blackout checkpoint is the
// strongest recovery preference, but it is on the far side of the league gate
// that stranded the run. Withholding it entirely would drop a legitimate heal
// the party still needs, so recovery must fall through to the next routable
// Center instead.
func TestRecoveryFallsBackToRoutableCenter(t *testing.T) {
	obs := Observation{
		GameID:             testGameID,
		Map:                1,
		Location:           "field",
		PartyCount:         1,
		Party:              []PartyMon{{Species: "testmon", Level: 20, HP: 8, MaxHP: 50}},
		RecoveryCheckpoint: "checkpoint pokemon center",
		Catalog: ObjectiveCatalog{Destinations: []CatalogDestination{
			{Place: "near pokemon center", Location: "near-center", Center: true},
			{Place: "checkpoint pokemon center", Location: "checkpoint-center", Center: true},
		}},
	}
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		"field":             {"near-center"},
		"near-center":       {"field", "checkpoint-center"},
		"checkpoint-center": {"near-center"},
	}})
	known.SawLocation("field")
	known.SawLocation("near-center")

	obs.Unroutable = []string{"checkpoint pokemon center"}
	got := OfferWithEvidence(obs, known).Candidates
	if !hasCatalogObjective(got, Objective{Kind: KindHeal, Place: "near pokemon center"}) {
		t.Fatalf("no fallback to a routable Center after the checkpoint was rejected: %+v", got)
	}
	if hasCatalogObjective(got, Objective{Kind: KindHeal, Place: "checkpoint pokemon center"}) {
		t.Fatalf("unroutable checkpoint still offered for travel: %+v", got)
	}
}

func TestRecoveryPrefersSuccessfulVisitedCheckpoint(t *testing.T) {
	obs := Observation{
		Location:   "field",
		PartyCount: 1,
		Party:      []PartyMon{{Species: "testmon", Level: 20, HP: 8, MaxHP: 50}},
		Catalog: ObjectiveCatalog{Destinations: []CatalogDestination{
			{Place: "near pokemon center", Location: "near-center", Center: true},
			{Place: "used pokemon center", Location: "used-center", Center: true},
		}},
	}
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		"field":       {"near-center"},
		"near-center": {"field", "used-center"},
		"used-center": {"near-center"},
	}})
	known.SawLocation("near-center")
	known.SawLocation("used-center")
	known.rememberRecoveryCheckpoint("used pokemon center", "used-center", true)

	got := OfferWithEvidence(obs, known).Candidates
	if !hasCatalogObjective(got, Objective{Kind: KindHeal, Place: "used pokemon center"}) {
		t.Fatalf("successful checkpoint was not preferred: %+v", got)
	}
	if hasCatalogObjective(got, Objective{Kind: KindHeal, Place: "near pokemon center"}) {
		t.Fatalf("closer visited-only center beat successful checkpoint: %+v", got)
	}
}

func TestNoteObservationLearnsCenterCheckpoint(t *testing.T) {
	obs := Observation{
		Location: "center-location",
		Catalog: ObjectiveCatalog{Destinations: []CatalogDestination{
			{Place: "test pokemon center", Location: "center-location", Center: true},
		}},
	}
	known := NewKnowledge(nil)
	noteObservation(known, obs)
	got, ok := known.RecoveryCheckpoints["test pokemon center"]
	if !ok || got.Location != "center-location" || got.Successful {
		t.Fatalf("learned checkpoint = %+v, ok=%v", got, ok)
	}

	obs.RecoveryCheckpoint = "test pokemon center"
	noteObservation(known, obs)
	got = known.RecoveryCheckpoints["test pokemon center"]
	if !got.Successful {
		t.Fatalf("active cartridge checkpoint did not strengthen learned checkpoint: %+v", got)
	}
}
