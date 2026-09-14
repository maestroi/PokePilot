package agent

import "testing"

func TestCompletionistRewardsMissingDexCatchWithFullParty(t *testing.T) {
	obs := Observation{
		PartyCount:   6,
		PokedexOwned: []SpeciesID{"charmander"},
		Dex: DexCatalog{
			Owned:   []DexEntry{{Species: "charmander", Owned: true}},
			Targets: []DexEntry{{Species: "rattata"}},
		},
	}
	profile := PlayStyle(PlayStyleCompletionist)
	catch := ScoreObjective(obs, Objective{Kind: KindCatch, Species: "rattata"}, profile)
	if !naturalSignalHasTag(catch.Natural, "new-dex-entry") {
		t.Fatalf("completionist catch natural signal = %+v, want new-dex-entry", catch.Natural)
	}

	progress := ScoreObjective(obs, Objective{Kind: KindProgress, Progress: ProgressID("test_progress")}, profile)
	if catch.Total <= progress.Total {
		t.Fatalf("full-party new Dex catch should beat generic progression: catch %.3f progress %.3f", catch.Total, progress.Total)
	}
}

func TestCompletionistRewardsMartWhenDexTargetsNeedCaptureStock(t *testing.T) {
	obs := Observation{
		Money:      200,
		PartyCount: 1,
		Dex:        DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
	}
	signal := naturalPlaySignal(obs, Objective{Kind: KindGoTo, Place: PlaceID("viridian mart")}, PlayStyle(PlayStyleCompletionist))
	if !naturalSignalHasTag(signal, "capture-resupply") {
		t.Fatalf("completionist Mart signal = %+v, want capture-resupply", signal)
	}
}

func TestCompletionistDoesNotRewardUnaffordableMartResupply(t *testing.T) {
	obs := Observation{
		Money:      187,
		PartyCount: 1,
		Dex:        DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
	}
	signal := naturalPlaySignal(obs, Objective{Kind: KindGoTo, Place: PlaceID("viridian mart")}, PlayStyle(PlayStyleCompletionist))
	if naturalSignalHasTag(signal, "capture-resupply") {
		t.Fatalf("completionist Mart signal = %+v, should not reward capture resupply with only ¥%d", signal, obs.Money)
	}
}

func TestCompletionistRewardsBuyingBallsForRemainingDexTargets(t *testing.T) {
	obs := Observation{Dex: DexCatalog{Targets: []DexEntry{{Species: "rattata"}}}}
	signal := naturalPlaySignal(obs, Objective{Kind: KindBuy, Item: ItemID("pokeball"), Qty: 5}, PlayStyle(PlayStyleCompletionist))
	if !naturalSignalHasTag(signal, "dex-supplies") {
		t.Fatalf("completionist ball purchase signal = %+v, want dex-supplies", signal)
	}
}

func TestDexCollectionBonusesStopWhenNothingRemains(t *testing.T) {
	obs := Observation{
		PartyCount:   6,
		PokedexOwned: []SpeciesID{"rattata"},
		Dex:          DexCatalog{Owned: []DexEntry{{Species: "rattata", Owned: true}}},
	}
	profile := PlayStyle(PlayStyleCompletionist)

	catch := naturalPlaySignal(obs, Objective{Kind: KindCatch, Species: "rattata"}, profile)
	if naturalSignalHasTag(catch, "new-dex-entry") {
		t.Fatalf("owned species still received new-dex-entry: %+v", catch)
	}

	mart := naturalPlaySignal(obs, Objective{Kind: KindGoTo, Place: PlaceID("viridian mart")}, profile)
	if naturalSignalHasTag(mart, "capture-resupply") {
		t.Fatalf("completed Dex still received capture-resupply: %+v", mart)
	}

	buy := naturalPlaySignal(obs, Objective{Kind: KindBuy, Item: ItemID("pokeball"), Qty: 5}, profile)
	if naturalSignalHasTag(buy, "dex-supplies") {
		t.Fatalf("completed Dex still received dex-supplies: %+v", buy)
	}
}

func naturalSignalHasTag(signal NaturalPlaySignal, want string) bool {
	for _, tag := range signal.Tags {
		if tag == want {
			return true
		}
	}
	return false
}
