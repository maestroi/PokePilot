package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestYellowObjectiveCatalogOffersSemanticPikachuStarter(t *testing.T) {
	provider, ok := objectiveCatalogProviderFor(yellowprofile.GameID)
	if !ok {
		t.Fatal("Yellow objective catalog provider not registered")
	}
	catalog := provider.ObjectiveCatalog(Observation{GameID: yellowprofile.GameID, PartyCount: 0})
	if len(catalog.Starters) != 1 || catalog.Starters[0].Species != game.SpeciesID("pikachu") {
		t.Fatalf("Yellow starters = %+v", catalog.Starters)
	}
	offer := OfferWithEvidence(Observation{GameID: yellowprofile.GameID, PartyCount: 0}, NewKnowledge(nil))
	if len(offer.Candidates) != 1 || offer.Candidates[0].Kind != KindStarter || offer.Candidates[0].Species != "pikachu" {
		t.Fatalf("Yellow opening offer = %+v", offer.Candidates)
	}
}

func TestYellowAdapterIsSeparateFromRedBlueFactory(t *testing.T) {
	factory := objectiveAdapterFactories[yellowprofile.GameID]
	if factory == nil {
		t.Fatal("Yellow objective adapter not registered")
	}
	if _, ok := factory(nil, nil, RoutePriorityConservative).(*yellowObjectiveAdapter); !ok {
		t.Fatalf("Yellow factory returned %T", factory(nil, nil, RoutePriorityConservative))
	}
}
