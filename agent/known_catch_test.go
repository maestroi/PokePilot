package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func knownCatchTestWild(t *testing.T) wildGrassLookup {
	t.Helper()
	pidgey, ok := redSpeciesID(SpeciesID("pidgey"))
	if !ok {
		t.Fatal("pidgey Red species id missing")
	}
	rattata, ok := redSpeciesID(SpeciesID("rattata"))
	if !ok {
		t.Fatal("rattata Red species id missing")
	}
	return func(_ []byte, _ uint8) ([]skill.WildSpecies, error) {
		return []skill.WildSpecies{{ID: pidgey}, {ID: rattata}}, nil
	}
}

func TestAppendKnownCatchObjectivesOffersVisitedHabitatAwayFromGrass(t *testing.T) {
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("route 1 place missing")
	}
	pewterCenter, ok := skill.Place("pewter pokemon center")
	if !ok {
		t.Fatal("pewter pokemon center place missing")
	}

	known := NewKnowledge(nil)
	known.SawMap(route1.Map)
	obs := Observation{
		Map:        pewterCenter.Map,
		MapName:    "PEWTER_POKECENTER",
		PartyCount: 1,
		Party:      []PartyMon{{Species: SpeciesID("charmander"), Level: 12}},
		Bag:        []Item{{Name: "pokeball", Quantity: 5}},
	}

	got := appendKnownCatchObjectivesWithWild(nil, obs, known, nil, knownCatchTestWild(t))
	if len(got) == 0 {
		t.Fatal("no known-habitat catch objectives offered")
	}
	found := false
	for _, o := range got {
		if o.Kind != KindCatch || o.Place != PlaceID("route 1") {
			continue
		}
		if o.Species != SpeciesID("pidgey") && o.Species != SpeciesID("rattata") {
			continue
		}
		if !o.Flee {
			t.Fatalf("remote catch objective = %+v, want flee travel", o)
		}
		if !strings.Contains(strings.ToLower(o.Note), "known habitat: route 1") {
			t.Fatalf("remote catch note = %q, want route 1 habitat context", o.Note)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("known-habitat catches = %+v, want Route 1 PIDGEY or RATTATA", got)
	}
}

func TestAppendKnownCatchObjectivesRequiresBallsAndOpenPartySlot(t *testing.T) {
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("route 1 place missing")
	}
	known := NewKnowledge(nil)
	known.SawMap(route1.Map)
	wildFor := knownCatchTestWild(t)

	base := Observation{Map: 0xff, PartyCount: 1, Party: []PartyMon{{Species: SpeciesID("charmander")}}}
	if got := appendKnownCatchObjectivesWithWild(nil, base, known, nil, wildFor); len(got) != 0 {
		t.Fatalf("without balls got %d catch objectives, want 0", len(got))
	}
	base.Bag = []Item{{Name: "pokeball", Quantity: 5}}
	base.PartyCount = 6
	if got := appendKnownCatchObjectivesWithWild(nil, base, known, nil, wildFor); len(got) != 0 {
		t.Fatalf("with full party got %d catch objectives, want 0", len(got))
	}
}

func TestAppendKnownCatchObjectivesDoesNotRevealUnvisitedHabitat(t *testing.T) {
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("route 1 place missing")
	}
	known := NewKnowledge(nil)
	obs := Observation{
		Map:        0xff,
		PartyCount: 1,
		Party:      []PartyMon{{Species: SpeciesID("charmander")}},
		Bag:        []Item{{Name: "pokeball", Quantity: 5}},
	}
	called := false
	wildFor := func(_ []byte, mapID uint8) ([]skill.WildSpecies, error) {
		called = true
		if mapID != route1.Map {
			t.Fatalf("wild lookup map = %#x, want Route 1 %#x", mapID, route1.Map)
		}
		return nil, nil
	}
	if got := appendKnownCatchObjectivesWithWild(nil, obs, known, nil, wildFor); len(got) != 0 {
		t.Fatalf("unvisited habitat produced %d catch objectives, want 0", len(got))
	}
	if called {
		t.Fatal("wild lookup called for an unvisited habitat")
	}
}
