package agent

import "testing"

func TestAppendDexFossilObjectivesOffersOwnedFossil(t *testing.T) {
	obs := Observation{
		Map: 0xAA,
		Bag: []Item{{Name: "helix fossil", Quantity: 1}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "omanyte",
			Sources: []DexSource{{Kind: AcquireFossil, Requirement: "helix_fossil"}},
		}}},
	}

	got := appendDexFossilObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("fossil objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "omanyte" || o.Place != "cinnabar lab fossil revival" || o.Intent != dexFossilIntent || !o.Flee {
		t.Fatalf("Omanyte fossil objective = %+v", o)
	}
}

func TestAppendDexFossilObjectivesRequiresFossilItem(t *testing.T) {
	obs := Observation{
		Map: 0xAA,
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "omanyte",
			Sources: []DexSource{{Kind: AcquireFossil, Requirement: "helix_fossil"}},
		}}},
	}
	if got := appendDexFossilObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("fossil objectives without Helix Fossil = %+v, want none", got)
	}
}

func TestAppendDexFossilObjectivesSuppressesOwnedTarget(t *testing.T) {
	obs := Observation{
		Map:          0xAA,
		Bag:          []Item{{Name: "helix fossil", Quantity: 1}},
		PokedexOwned: []SpeciesID{"omanyte"},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "omanyte",
			Sources: []DexSource{{Kind: AcquireFossil, Requirement: "helix_fossil"}},
		}}},
	}
	if got := appendDexFossilObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("owned Omanyte produced fossil objective: %+v", got)
	}
}

func TestAppendDexFossilObjectivesSupportsOldAmber(t *testing.T) {
	obs := Observation{
		Map: 0xAA,
		Bag: []Item{{Name: "old amber", Quantity: 1}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "aerodactyl",
			Sources: []DexSource{{Kind: AcquireFossil, Requirement: "old_amber"}},
		}}},
	}
	got := appendDexFossilObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 || got[0].Species != "aerodactyl" || got[0].Intent != dexFossilIntent {
		t.Fatalf("Old Amber objective = %+v, want Aerodactyl revival", got)
	}
}

func TestFossilScientistIsOwnedSemanticInteraction(t *testing.T) {
	if !redOwnedChoiceActor(0xAA, 5, 2) {
		t.Fatal("Cinnabar fossil scientist should be suppressed from generic Talk")
	}
}
