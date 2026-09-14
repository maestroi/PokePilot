package agent

import "testing"

func staticDexObservation(species SpeciesID, mapID uint8, requirement string) Observation {
	return Observation{
		Map: mapID,
		Bag: []Item{{Name: "ultra ball", Quantity: 5}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: species,
			Sources: []DexSource{{Kind: AcquireStatic, Requirement: requirement}},
		}}},
	}
}

func TestAppendDexStaticObjectivesOffersReachableZapdos(t *testing.T) {
	obs := staticDexObservation("zapdos", 0x53, "")
	got := appendDexStaticObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("static objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "zapdos" || o.Place != "power plant zapdos" || o.Intent != dexStaticIntent || !o.Flee {
		t.Fatalf("Zapdos objective = %+v", o)
	}
}

func TestAppendDexStaticObjectivesRequiresPokeFluteForSnorlax(t *testing.T) {
	obs := staticDexObservation("snorlax", 0x1B, "poke_flute")
	if got := appendDexStaticObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("Snorlax without flute = %+v, want none", got)
	}
	obs.Bag = append(obs.Bag, Item{Name: "poke flute", Quantity: 1})
	got := appendDexStaticObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 || got[0].Species != "snorlax" || got[0].Place != "route 16 snorlax capture" {
		t.Fatalf("Snorlax with flute = %+v, want route 16 static objective", got)
	}
}

func TestAppendDexStaticObjectivesRequiresBalls(t *testing.T) {
	obs := staticDexObservation("articuno", 0xA2, "")
	obs.Bag = nil
	if got := appendDexStaticObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("static without balls = %+v, want none", got)
	}
}

func TestAppendDexStaticObjectivesSuppressesOwnedTarget(t *testing.T) {
	obs := staticDexObservation("moltres", 0xC2, "")
	obs.PokedexOwned = []SpeciesID{"moltres"}
	if got := appendDexStaticObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("owned Moltres = %+v, want none", got)
	}
}

func TestAppendDexStaticObjectivesQuarantinesConsumedOrExhaustedSource(t *testing.T) {
	obs := staticDexObservation("mewtwo", 0xE3, "")
	known := NewKnowledge(nil)
	first := appendDexStaticObjectives(obs, known, nil)
	if len(first) != 1 {
		t.Fatalf("initial Mewtwo objective = %+v, want one", first)
	}
	key := first[0].String()
	known.Failures[key] = Failure{Objective: key, Times: 1, Last: "skill: static capture attempts exhausted: Mewtwo after 6 rollback-safe phases"}
	if got := appendDexStaticObjectives(obs, known, nil); len(got) != 0 {
		t.Fatalf("quarantined Mewtwo = %+v, want none", got)
	}
}
