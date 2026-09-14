package agent

import "testing"

func porygonDexObservation(money uint32) Observation {
	return Observation{
		Map:   0x89,
		Money: money,
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "porygon",
			Sources: []DexSource{{Kind: AcquireGift, Requirement: "game_corner"}},
		}}},
	}
}

func TestAppendDexGameCornerObjectivesOffersFundedPorygon(t *testing.T) {
	obs := porygonDexObservation(200000)
	got := appendDexGameCornerObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("Game Corner objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "porygon" || o.Place != "game corner porygon prize" || o.Intent != dexGameCornerIntent || !o.Flee {
		t.Fatalf("Porygon objective = %+v", o)
	}
}

func TestAppendDexGameCornerObjectivesSuppressesUnfundedPorygon(t *testing.T) {
	obs := porygonDexObservation(199999)
	if got := appendDexGameCornerObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("unfunded Porygon objectives = %+v, want none", got)
	}
}

func TestAppendDexGameCornerObjectivesSuppressesOwnedPorygon(t *testing.T) {
	obs := porygonDexObservation(200000)
	obs.PokedexOwned = []SpeciesID{"porygon"}
	if got := appendDexGameCornerObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("owned Porygon objectives = %+v, want none", got)
	}
}
