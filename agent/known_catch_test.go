package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

func TestAppendKnownCatchObjectivesOffersVisitedHabitatAwayFromGrass(t *testing.T) {
	e := fixture.Load(t, "post_pokeballs")
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

	got := appendKnownCatchObjectives(e.ROM(), obs, known, nil)
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
	e := fixture.Load(t, "post_pokeballs")
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("route 1 place missing")
	}
	known := NewKnowledge(nil)
	known.SawMap(route1.Map)

	base := Observation{Map: 0xff, PartyCount: 1, Party: []PartyMon{{Species: SpeciesID("charmander")}}}
	if got := appendKnownCatchObjectives(e.ROM(), base, known, nil); len(got) != 0 {
		t.Fatalf("without balls got %d catch objectives, want 0", len(got))
	}
	base.Bag = []Item{{Name: "pokeball", Quantity: 5}}
	base.PartyCount = 6
	if got := appendKnownCatchObjectives(e.ROM(), base, known, nil); len(got) != 0 {
		t.Fatalf("with full party got %d catch objectives, want 0", len(got))
	}
}
