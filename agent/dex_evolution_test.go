package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestAppendDexEvolutionObjectivesOffersLevelEvolutionForPartyBase(t *testing.T) {
	obs := Observation{
		HasGrass: true,
		Party: []PartyMon{{
			Species: "caterpie",
			Level:   6,
			HP:      20,
			MaxHP:   20,
		}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "metapod",
			Sources: []DexSource{{Kind: AcquireLevelEvo, From: "caterpie", Level: 7}},
		}}},
	}

	got := appendDexEvolutionObjectives(obs, nil, nil)
	if len(got) != 1 {
		t.Fatalf("evolution objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindTrain || o.Species != "caterpie" || o.Level != 7 || o.Slot != 0 || o.Intent != "dex-evolution" {
		t.Fatalf("objective = %+v, want targeted Caterpie training to level 7", o)
	}
}

func TestAppendDexEvolutionObjectivesRetriesCancelledLevelEvolutionAtNextLevel(t *testing.T) {
	obs := Observation{
		HasGrass: true,
		Party:    []PartyMon{{Species: "caterpie", Level: 7, HP: 20, MaxHP: 20}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "metapod",
			Sources: []DexSource{{Kind: AcquireLevelEvo, From: "caterpie", Level: 7}},
		}}},
	}
	got := appendDexEvolutionObjectives(obs, nil, nil)
	if len(got) != 1 || got[0].Level != 8 {
		t.Fatalf("cancelled evolution retry = %+v, want next level 8", got)
	}
}

func TestAppendDexEvolutionObjectivesOffersOwnedStoneOnCorrectSlot(t *testing.T) {
	obs := Observation{
		Party: []PartyMon{
			{Species: "pikachu", Level: 20, HP: 40, MaxHP: 40},
			{Species: "nidorino", Level: 22, HP: 50, MaxHP: 50},
		},
		Bag: []Item{{Name: "moon stone", Quantity: 1}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "nidoking",
			Sources: []DexSource{{Kind: AcquireItemEvo, From: "nidorino", Item: "moon stone"}},
		}}},
	}

	got := appendDexEvolutionObjectives(obs, nil, nil)
	if len(got) != 1 {
		t.Fatalf("stone evolution objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindUseItem || o.Item != "moon stone" || o.Slot != 1 || o.Intent != "dex-evolution" {
		t.Fatalf("objective = %+v, want Moon Stone on Nidorino slot 1", o)
	}
}

func TestAppendDexEvolutionObjectivesBuysMissingPurchasableStone(t *testing.T) {
	dest, ok := skill.Place("celadon mart 4f stones")
	if !ok {
		t.Fatal("Celadon Mart 4F evolution stone destination missing")
	}
	obs := Observation{
		Map:   dest.Map,
		Money: 2100,
		Party: []PartyMon{{Species: "pikachu", Level: 20, HP: 40, MaxHP: 40}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "raichu",
			Sources: []DexSource{{Kind: AcquireItemEvo, From: "pikachu", Item: "thunder stone"}},
		}}},
	}

	got := appendDexEvolutionObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("missing-stone objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindBuy || o.Item != "thunder stone" || o.Qty != 1 || o.Intent != dexEvolutionSupplyIntent {
		t.Fatalf("missing-stone objective = %+v, want one Thunder Stone purchase", o)
	}
}

func TestAppendDexEvolutionObjectivesDoesNotPretendMoonStoneIsSold(t *testing.T) {
	dest, ok := skill.Place("celadon mart 4f stones")
	if !ok {
		t.Fatal("Celadon Mart 4F evolution stone destination missing")
	}
	obs := Observation{
		Map:   dest.Map,
		Money: 99999,
		Party: []PartyMon{{Species: "nidorino", Level: 22, HP: 50, MaxHP: 50}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "nidoking",
			Sources: []DexSource{{Kind: AcquireItemEvo, From: "nidorino", Item: "moon stone"}},
		}}},
	}
	if got := appendDexEvolutionObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("missing Moon Stone objectives = %+v, want none because Mart does not sell Moon Stone", got)
	}
}

func TestAppendDexEvolutionObjectivesPreservesFuchsiaMoneyReserve(t *testing.T) {
	dest, ok := skill.Place("celadon mart 4f stones")
	if !ok {
		t.Fatal("Celadon Mart 4F evolution stone destination missing")
	}
	obs := Observation{
		Map:   dest.Map,
		Money: 2100,
		Bag:   []Item{{Name: "poke flute", Quantity: 1}},
		Party: []PartyMon{{Species: "pikachu", Level: 20, HP: 40, MaxHP: 40}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "raichu",
			Sources: []DexSource{{Kind: AcquireItemEvo, From: "pikachu", Item: "thunder stone"}},
		}}},
	}
	if got := appendDexEvolutionObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("stone purchase that spends reserved Safari money = %+v, want none", got)
	}
}

func TestAppendDexEvolutionObjectivesRequiresImmediatePrerequisiteAndSuppressesOwned(t *testing.T) {
	obs := Observation{
		HasGrass:     true,
		PokedexOwned: []SpeciesID{"metapod"},
		Party:        []PartyMon{{Species: "caterpie", Level: 6, HP: 20, MaxHP: 20}},
		Dex: DexCatalog{
			Owned: []DexEntry{{Species: "metapod"}},
			Targets: []DexEntry{
				{Species: "metapod", Sources: []DexSource{{Kind: AcquireLevelEvo, From: "caterpie", Level: 7}}},
				{Species: "raichu", Sources: []DexSource{{Kind: AcquireItemEvo, From: "pikachu", Item: "thunder stone"}}},
			},
		},
	}
	if got := appendDexEvolutionObjectives(obs, nil, nil); len(got) != 0 {
		t.Fatalf("unavailable/already-owned evolution objectives = %+v, want none", got)
	}
}
