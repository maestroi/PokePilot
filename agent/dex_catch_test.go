package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestAppendDexCatchObjectivesHuntsCatalogGrassNotJustVisitedMaps(t *testing.T) {
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("route 1 place missing")
	}
	obs := Observation{
		Map:        0xff,
		PartyCount: 6,
		Party:      []PartyMon{{Species: "charmander"}},
		Bag:        []Item{{Name: "pokeball", Quantity: 5}},
		Dex: DexCatalog{
			Targets: []DexEntry{{
				Species: "pidgey",
				Sources: []DexSource{
					{Kind: AcquireWildGrass, Place: "route 1"},
					{Kind: AcquireFishing, Place: "route 6", Requirement: "old_rod"},
				},
			}},
		},
	}

	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("dex catches = %+v, want one grass hunt", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "pidgey" || o.Place != "route 1" || !o.Flee {
		t.Fatalf("objective = %+v, want flee catch pidgey on route 1", o)
	}
	if !strings.Contains(strings.ToLower(o.Note), "dex target: route 1") {
		t.Fatalf("note = %q, want catalog habitat context", o.Note)
	}
	if dest, ok := skill.Place(string(o.Place)); !ok || dest.Map != route1.Map {
		t.Fatalf("place %q did not resolve to Route 1", o.Place)
	}
}

func TestAppendDexCatchObjectivesSkipsOwnedUnexecutableAndBlockedSources(t *testing.T) {
	obs := Observation{
		Map:          0xff,
		Bag:          []Item{{Name: "pokeball", Quantity: 5}},
		PokedexOwned: []SpeciesID{"pidgey"},
		Unroutable:   []string{"route 4"},
		Dex: DexCatalog{
			Owned: []DexEntry{{Species: "pidgey"}},
			Targets: []DexEntry{
				{Species: "pidgey", Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}}},
				{Species: "magikarp", Sources: []DexSource{{Kind: AcquireFishing, Place: "route 6", Requirement: "old_rod"}}},
				{Species: "ekans", Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 4"}}},
				{Species: "spearow", Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 3"}}},
			},
		},
	}

	got := appendDexCatchObjectives(obs, NewKnowledge(nil), []Objective{{Kind: KindCatch, Species: "rattata"}})
	for _, o := range got {
		switch o.Species {
		case "pidgey", "magikarp", "ekans", "spearow":
			t.Fatalf("offered blocked/unexecutable/owned %s: %+v", o.Species, got)
		}
	}
}

func TestAppendDexCatchObjectivesPicksNearestSourceAndStaysBounded(t *testing.T) {
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("route 1 place missing")
	}
	route2, ok := skill.Place("route 2")
	if !ok {
		t.Fatal("route 2 place missing")
	}
	obs := Observation{
		Map: route1.Map,
		Bag: []Item{{Name: "pokeball", Quantity: 3}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "pidgey",
			Sources: []DexSource{
				{Kind: AcquireWildGrass, Place: "route 2"},
				{Kind: AcquireWildGrass, Place: "route 1"},
			},
		}}},
	}
	known := NewKnowledge(map[uint8][]uint8{route1.Map: {route2.Map}, route2.Map: {route1.Map}})
	got := appendDexCatchObjectives(obs, known, nil)
	if len(got) != 1 || got[0].Place != "route 1" {
		t.Fatalf("nearest habitat = %+v, want route 1", got)
	}

	obs.Dex.Targets = make([]DexEntry, 0, dexCatchLimit+2)
	for i := 0; i < dexCatchLimit+2; i++ {
		obs.Dex.Targets = append(obs.Dex.Targets, DexEntry{
			Species: SpeciesID("extra" + string(rune('a'+i))),
			Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}},
		})
	}
	got = appendDexCatchObjectives(obs, known, nil)
	if len(got) != dexCatchLimit {
		t.Fatalf("bounded catches = %d, want %d", len(got), dexCatchLimit)
	}
}

func TestAppendDexCatchObjectivesRequiresBalls(t *testing.T) {
	obs := Observation{
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "pidgey",
			Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}},
		}}},
	}
	if got := appendDexCatchObjectives(obs, nil, nil); len(got) != 0 {
		t.Fatalf("without balls got %+v", got)
	}
}
