package agent

import "testing"

func TestYellowScriptedDexSourcesUsePikachuStarterAndIndependentKantoGifts(t *testing.T) {
	sources := yellowScriptedDexSources()
	kinds := map[SpeciesID]DexSource{}
	for _, source := range sources {
		id := semanticSpeciesFromRed(source.internal)
		if id != "" {
			kinds[id] = source.source
		}
	}
	if got := kinds["pikachu"]; got.Kind != AcquireStarter || got.ExclusiveGroup != "" {
		t.Fatalf("Pikachu source = %+v", got)
	}
	for _, id := range []SpeciesID{"bulbasaur", "charmander", "squirtle"} {
		got := kinds[id]
		if got.Kind != AcquireGift || got.ExclusiveGroup != "" {
			t.Fatalf("%s source = %+v, want independent gift", id, got)
		}
	}
}

func TestYellowDexIncompleteModelCannotFalsePositiveCompletion(t *testing.T) {
	obs := Observation{Dex: DexCatalog{
		Owned:            []DexEntry{{Species: "pikachu", Owned: true}},
		Unavailable:      []DexEntry{{Species: "mew", Unavailable: UnavailableEventOnly}},
		IncompleteReason: yellowDexIncompleteReason,
	}}
	status := EvaluateGoal(Goal{Kind: GoalDex}, obs)
	if status.Complete {
		t.Fatalf("incomplete Yellow Dex model reported completion: %+v", status)
	}
	if status.Target != 1 || status.Current != 1 {
		t.Fatalf("incomplete Yellow Dex counters = %+v", status)
	}
}

func TestYellowEventOnlyPolicyDoesNotUseRedAssemblyDefault(t *testing.T) {
	species := []DexEntry{
		{Species: "mew", Dex: 151},
		{Species: "pikachu", Dex: 25},
	}
	cat := assembleDexCatalogWithPolicy(species, map[SpeciesID][]DexSource{}, nil, nil, nil, yellowEventOnly())
	if len(cat.Unavailable) != 2 {
		t.Fatalf("unavailable = %+v", cat.Unavailable)
	}
	if cat.Unavailable[0].Species == "mew" && cat.Unavailable[0].Unavailable != UnavailableEventOnly {
		t.Fatalf("Mew policy = %+v", cat.Unavailable[0])
	}
	for _, entry := range cat.Unavailable {
		if entry.Species == "mew" && entry.Unavailable != UnavailableEventOnly {
			t.Fatalf("Mew policy = %+v", entry)
		}
		if entry.Species == "pikachu" && entry.Unavailable != UnavailableNoLocalSource {
			t.Fatalf("Pikachu policy = %+v", entry)
		}
	}
}
