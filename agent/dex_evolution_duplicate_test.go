package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestAppendDexEvolutionObjectivesAcquiresRepeatableDuplicateBaseForBranch(t *testing.T) {
	dest, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("route 1 place missing")
	}
	obs := Observation{
		Map:          dest.Map,
		Party:        []PartyMon{{Species: "pikachu", Level: 20, HP: 40, MaxHP: 40}},
		Bag:          []Item{{Name: "pokeball", Quantity: 5}},
		PokedexOwned: []SpeciesID{"oddish", "branch-a"},
		Dex: DexCatalog{
			Owned: []DexEntry{
				{
					Species: "oddish",
					Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}},
				},
				{
					Species: "branch-a",
					Sources: []DexSource{{Kind: AcquireItemEvo, From: "oddish", Item: "leaf stone"}},
				},
			},
			Targets: []DexEntry{{
				Species: "branch-b",
				Sources: []DexSource{{Kind: AcquireItemEvo, From: "oddish", Item: "water stone"}},
			}},
		},
	}

	got := appendDexEvolutionObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("branched evolution objectives = %+v, want one duplicate-base catch", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "oddish" || o.Place != "route 1" || !o.Flee {
		t.Fatalf("duplicate-base objective = %+v", o)
	}
	if !strings.Contains(strings.ToLower(o.Note), "duplicate base") {
		t.Fatalf("duplicate-base note = %q", o.Note)
	}
}

func TestAppendDexEvolutionObjectivesDoesNotDuplicateBaseAlreadyInParty(t *testing.T) {
	dest, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("route 1 place missing")
	}
	obs := Observation{
		Map:          dest.Map,
		Party:        []PartyMon{{Species: "oddish", Level: 20, HP: 40, MaxHP: 40}},
		Bag:          []Item{{Name: "pokeball", Quantity: 5}},
		PokedexOwned: []SpeciesID{"oddish", "branch-a"},
		Dex: DexCatalog{
			Owned: []DexEntry{
				{Species: "oddish", Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}}},
				{Species: "branch-a", Sources: []DexSource{{Kind: AcquireItemEvo, From: "oddish", Item: "moon stone"}}},
			},
			Targets: []DexEntry{{
				Species: "branch-b",
				Sources: []DexSource{{Kind: AcquireItemEvo, From: "oddish", Item: "moon stone"}},
			}},
		},
	}

	if got := appendDexEvolutionObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("base already in party objectives = %+v, want no duplicate catch", got)
	}
}

func TestAppendDexEvolutionObjectivesDoesNotRepeatOneTimeGiftBase(t *testing.T) {
	obs := Observation{
		Party:        []PartyMon{{Species: "pikachu", Level: 20, HP: 40, MaxHP: 40}},
		Bag:          []Item{{Name: "pokeball", Quantity: 5}},
		PokedexOwned: []SpeciesID{"eevee", "vaporeon"},
		Dex: DexCatalog{
			Owned: []DexEntry{
				{Species: "eevee", Sources: []DexSource{{Kind: AcquireGift, Place: "celadon mansion eevee"}}},
				{Species: "vaporeon", Sources: []DexSource{{Kind: AcquireItemEvo, From: "eevee", Item: "water stone"}}},
			},
			Targets: []DexEntry{{
				Species: "jolteon",
				Sources: []DexSource{{Kind: AcquireItemEvo, From: "eevee", Item: "thunder stone"}},
			}},
		},
	}

	if got := appendDexEvolutionObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("one-time Eevee base objectives = %+v, want no fabricated duplicate", got)
	}
}

func TestAssembleDexCatalogForfeitsOtherEeveeBranchesAfterEvolution(t *testing.T) {
	species := []DexEntry{
		{Species: "eevee", Dex: 133},
		{Species: "vaporeon", Dex: 134},
		{Species: "jolteon", Dex: 135},
		{Species: "flareon", Dex: 136},
	}
	sources := map[SpeciesID][]DexSource{
		"eevee":    {{Kind: AcquireGift, Place: "celadon mansion eevee"}},
		"vaporeon": {{Kind: AcquireItemEvo, From: "eevee", Item: "water stone"}},
		"jolteon":  {{Kind: AcquireItemEvo, From: "eevee", Item: "thunder stone"}},
		"flareon":  {{Kind: AcquireItemEvo, From: "eevee", Item: "fire stone"}},
	}

	// Eevee's Pokédex bit remains owned after evolution; that historical bit
	// must not be mistaken for another physical Eevee individual.
	cat := assembleDexCatalog(species, sources, []SpeciesID{"eevee", "vaporeon"}, nil, redExclusiveChoices())
	for _, id := range []SpeciesID{"jolteon", "flareon"} {
		entry, ok := findDex(cat.Unavailable, id)
		if !ok || entry.Unavailable != UnavailableForfeited+":eevee_stone" {
			t.Fatalf("%s = %+v, want forfeited eevee_stone", id, entry)
		}
	}
}
