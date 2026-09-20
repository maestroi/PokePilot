package agent

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

type catalogTestPlanner struct {
	catalog ObjectiveCatalog
}

func (catalogTestPlanner) ProgressionObjectives(Observation) []Objective { return nil }
func (p catalogTestPlanner) ObjectiveCatalog(Observation) ObjectiveCatalog {
	return p.catalog
}

func hasCatalogObjective(objs []Objective, want Objective) bool {
	for _, got := range objs {
		if got.Kind == want.Kind && got.Place == want.Place && got.Species == want.Species &&
			got.Item == want.Item && got.X == want.X && got.Y == want.Y && got.Starter == want.Starter {
			return true
		}
	}
	return false
}

func TestAdapterCatalogFeedsGenericProviders(t *testing.T) {
	obs := Observation{
		Map:        1,
		Location:   "alpha",
		MapName:    "ALPHA",
		PartyCount: 1,
		Party:      []PartyMon{{Species: "testmon", Level: 10, HP: 30, MaxHP: 30}},
		HasGrass:   true,
		WildGrass:  []WildSpecies{{Name: "ignored-red-name", MinLevel: 2, MaxLevel: 4, Slots: 1}},
		Bag:        []Item{{Name: "pokeball", Quantity: 1}},
		Money:      3000,
		MartStock:  []string{"pokeball"},
	}
	catalog := ObjectiveCatalog{
		Destinations: []CatalogDestination{{Place: "beta", Location: "beta", X: 3, Y: 4}},
		Challenges:   []CatalogChallenge{{Place: "alpha gym", Location: "alpha"}},
		LocalEncounters: []CatalogEncounter{{
			Species: "catalogmon", MinLevel: 2, MaxLevel: 4, Slots: 1,
		}},
		Shop: &CatalogShop{Items: []CatalogShopItem{{Item: "pokeball", Name: "pokeball"}}},
		Interactables: []CatalogInteractable{
			{Kind: CatalogInteractablePerson, X: 4, Y: 5},
			{Kind: CatalogInteractableTrainer, X: 6, Y: 7, Challengeable: true},
			{Kind: CatalogInteractableItem, X: 8, Y: 9, Item: "potion"},
		},
	}
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		"alpha": {"beta"},
		"beta":  {"alpha"},
	}})
	known.SawLocation("alpha")

	got := OfferWithProgressionEvidence(obs, known, catalogTestPlanner{catalog: catalog}).Candidates
	for _, want := range []Objective{
		{Kind: KindCatch, Species: "catalogmon"},
		{Kind: KindGym, Place: "alpha gym"},
		{Kind: KindTalk, X: 4, Y: 5},
		{Kind: KindTrainer, X: 6, Y: 7},
		{Kind: KindPickup, X: 8, Y: 9, Item: "potion"},
		{Kind: KindGoTo, Place: "beta"},
	} {
		if !hasCatalogObjective(got, want) {
			t.Fatalf("catalog objective %+v missing from offer: %+v", want, got)
		}
	}
	if hasCatalogObjective(got, Objective{Kind: KindCatch, Species: "ignored-red-name"}) {
		t.Fatalf("provider reconstructed encounter from observation instead of adapter catalog: %+v", got)
	}
}

func TestAdapterCatalogOwnsStarterChoices(t *testing.T) {
	obs := Observation{Map: 1, PartyCount: 0}
	planner := catalogTestPlanner{catalog: ObjectiveCatalog{Starters: []CatalogStarter{
		{Starter: skill.StarterSquirtle, Species: "squirtle"},
	}}}
	got := OfferWithProgressionEvidence(obs, NewKnowledge(nil), planner).Candidates
	if !hasCatalogObjective(got, Objective{Kind: KindStarter, Starter: skill.StarterSquirtle, Species: "squirtle"}) {
		t.Fatalf("adapter starter missing: %+v", got)
	}
	if hasCatalogObjective(got, Objective{Kind: KindStarter, Starter: skill.StarterCharmander}) ||
		hasCatalogObjective(got, Objective{Kind: KindStarter, Starter: skill.StarterBulbasaur}) {
		t.Fatalf("generic provider invented Red starter choices: %+v", got)
	}
}

func TestCatalogCurrentCenterControlsRecoveryProvider(t *testing.T) {
	obs := Observation{
		Map: 1, PartyCount: 1,
		Party: []PartyMon{{Species: "testmon", Level: 10, HP: 5, MaxHP: 30}},
	}
	obs.Catalog = ObjectiveCatalog{CurrentCenter: true}
	got := OfferWithEvidence(obs, NewKnowledge(nil)).Candidates
	if !hasCatalogObjective(got, Objective{Kind: KindHeal}) {
		t.Fatalf("semantic current-center fact did not offer local heal: %+v", got)
	}
}

func TestCatalogNormalizationKeepsOffersDeterministic(t *testing.T) {
	obs := Observation{Map: 1, Location: "alpha", PartyCount: 1}
	obs.Catalog = ObjectiveCatalog{Destinations: []CatalogDestination{
		{Place: "zeta", Location: "beta"},
		{Place: "beta", Location: "beta"},
	}}
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		"alpha": {"beta"},
	}})
	first := OfferWithEvidence(obs, known)
	second := OfferWithEvidence(obs, known)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("catalog-backed provider pipeline is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}
	travel := []PlaceID{}
	for _, objective := range first.Candidates {
		if objective.Kind == KindGoTo && !objective.Flee {
			travel = append(travel, objective.Place)
		}
	}
	if !reflect.DeepEqual(travel, []PlaceID{"beta", "zeta"}) {
		t.Fatalf("travel order = %v, want adapter-independent semantic order", travel)
	}
}
