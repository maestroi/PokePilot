package agent

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func hallOfFameDexObservation(targets ...DexEntry) Observation {
	return Observation{
		Location: "route 15",
		Story: ProgressState{
			{ID: ProgressMainStoryComplete, Complete: true},
		},
		Dex: DexCatalog{Targets: targets},
	}
}

func TestDexCleanupDoesNotChangePreHallOfFameOffers(t *testing.T) {
	obs := Observation{
		Dex: DexCatalog{Targets: []DexEntry{{Species: "vaporeon"}}},
		Party: []PartyMon{{Species: "eevee", Level: 25}},
	}
	offered := []Objective{
		{Kind: KindCatch, Species: "vaporeon", Place: "route 10"},
		{Kind: KindUseItem, Item: "water stone", Slot: 0, Intent: "dex-evolution"},
	}
	got := prioritizeDexCleanupObjectives(obs, nil, offered)
	if !reflect.DeepEqual(got, offered) {
		t.Fatalf("pre-Hall-of-Fame cleanup changed offers:\n got: %+v\nwant: %+v", got, offered)
	}
}

func TestDexCleanupPrefersImmediateRepeatableStoneEvolutionOverRemoteCatch(t *testing.T) {
	obs := hallOfFameDexObservation(DexEntry{
		Species: "vaporeon",
		Sources: []DexSource{
			{Kind: AcquireWildGrass, Place: "route 10"},
			{Kind: AcquireItemEvo, From: "eevee", Item: "water stone"},
		},
	})
	obs.Party = []PartyMon{{Species: "eevee", Level: 25}}
	offered := []Objective{
		{Kind: KindHeal},
		{Kind: KindCatch, Species: "vaporeon", Place: "route 10"},
		{Kind: KindUseItem, Item: "water stone", Slot: 0, Intent: "dex-evolution"},
	}

	got := prioritizeDexCleanupObjectives(obs, nil, offered)
	if len(got) != 2 {
		t.Fatalf("cleanup offers = %+v, want heal + one acquisition route", got)
	}
	if got[0].Kind != KindHeal || got[1].Kind != KindUseItem {
		t.Fatalf("cleanup offers = %+v, want heal passthrough then stone evolution", got)
	}
	if !strings.Contains(got[1].Note, "VAPOREON via evolution_item") {
		t.Fatalf("cleanup annotation = %q", got[1].Note)
	}
}

func TestDexCleanupPrefersLocalDirectCatchOverLongTraining(t *testing.T) {
	obs := hallOfFameDexObservation(DexEntry{
		Species: "pidgeotto",
		Sources: []DexSource{
			{Kind: AcquireWildGrass, Place: "route 15"},
			{Kind: AcquireLevelEvo, From: "pidgey", Level: 18},
		},
	})
	obs.Party = []PartyMon{{Species: "pidgey", Level: 5}}
	offered := []Objective{
		{Kind: KindCatch, Species: "pidgeotto", Place: "route 15"},
		{Kind: KindTrain, Species: "pidgey", Slot: 0, Level: 18, Intent: "dex-evolution"},
	}

	got := prioritizeDexCleanupObjectives(obs, nil, offered)
	if len(got) != 1 || got[0].Kind != KindCatch || got[0].Species != "pidgeotto" {
		t.Fatalf("cleanup offers = %+v, want local direct Pidgeotto catch", got)
	}
}

func TestDexCleanupConservesFiniteMoonStoneWhenDirectCatchIsLocal(t *testing.T) {
	obs := hallOfFameDexObservation(DexEntry{
		Species: "clefable",
		Sources: []DexSource{
			{Kind: AcquireWildGrass, Place: "route 15"},
			{Kind: AcquireItemEvo, From: "clefairy", Item: "moon stone"},
		},
	})
	obs.Party = []PartyMon{{Species: "clefairy", Level: 20}}
	offered := []Objective{
		{Kind: KindCatch, Species: "clefable", Place: "route 15"},
		{Kind: KindUseItem, Item: "moon stone", Slot: 0, Intent: "dex-evolution"},
	}

	got := prioritizeDexCleanupObjectives(obs, nil, offered)
	if len(got) != 1 || got[0].Kind != KindCatch {
		t.Fatalf("cleanup offers = %+v, want direct catch before consuming a finite Moon Stone", got)
	}
}

func TestDexCleanupBoundsAcquisitionWindowAndKeepsEnablers(t *testing.T) {
	targets := make([]DexEntry, 0, dexCleanupObjectiveLimit+2)
	offered := []Objective{{Kind: KindBuy, Item: "water stone", Qty: 1, Intent: dexEvolutionSupplyIntent}}
	for i := 0; i < dexCleanupObjectiveLimit+2; i++ {
		id := SpeciesID(fmt.Sprintf("species-%02d", i))
		targets = append(targets, DexEntry{Species: id, Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 15"}}})
		offered = append(offered, Objective{Kind: KindCatch, Species: id, Place: "route 15"})
	}
	obs := hallOfFameDexObservation(targets...)

	got := prioritizeDexCleanupObjectives(obs, nil, offered)
	if len(got) != dexCleanupObjectiveLimit+1 {
		t.Fatalf("cleanup offer count = %d, want %d (one enabler + bounded acquisitions): %+v", len(got), dexCleanupObjectiveLimit+1, got)
	}
	if got[0].Kind != KindBuy || got[0].Intent != dexEvolutionSupplyIntent {
		t.Fatalf("cleanup dropped/reordered enabler: %+v", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i].Kind != KindCatch {
			t.Fatalf("cleanup acquisition %d = %+v, want catch", i, got[i])
		}
	}
}
