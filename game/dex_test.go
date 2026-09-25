package game

import "testing"

func TestAssembleDexCatalogKeepsProfileFactsPortable(t *testing.T) {
	species := []DexEntry{
		{Species: "starter-a", Dex: 1},
		{Species: "starter-b", Dex: 2},
		{Species: "starter-b-evo", Dex: 3},
		{Species: "event-only", Dex: 4},
		{Species: "trade-only", Dex: 5},
	}
	sources := map[SpeciesID][]DexSource{
		"starter-a":     {{Kind: AcquireStarter, ExclusiveGroup: "starter"}},
		"starter-b":     {{Kind: AcquireStarter, ExclusiveGroup: "starter"}},
		"starter-b-evo": {{Kind: AcquireLevelEvo, From: "starter-b", Level: 16}},
		"trade-only":    {{Kind: AcquireTradeEvo, From: "middle"}},
	}
	choices := []DexExclusiveChoice{{
		Group: "starter",
		Alternatives: [][]SpeciesID{{"starter-a"}, {"starter-b", "starter-b-evo"}},
	}}
	cat := AssembleDexCatalog(species, sources, []SpeciesID{"starter-a"}, nil, choices, map[SpeciesID]bool{"event-only": true})

	if len(cat.Owned) != 1 || cat.Owned[0].Species != "starter-a" {
		t.Fatalf("owned = %+v, want starter-a", cat.Owned)
	}
	wantUnavailable := map[SpeciesID]string{
		"starter-b":     UnavailableForfeited + ":starter",
		"starter-b-evo": UnavailableForfeited + ":starter",
		"event-only":    UnavailableEventOnly,
		"trade-only":    UnavailableTradeEvolution,
	}
	for id, reason := range wantUnavailable {
		found := false
		for _, entry := range cat.Unavailable {
			if entry.Species == id {
				found = true
				if entry.Unavailable != reason {
					t.Fatalf("%s unavailable = %q, want %q", id, entry.Unavailable, reason)
				}
			}
		}
		if !found {
			t.Fatalf("%s missing from unavailable: %+v", id, cat.Unavailable)
		}
	}
}
