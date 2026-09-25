package agent

import (
	"errors"
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

func TestYellowUnownedStoryGoalIsTypedBlockNotRedScript(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	o := Objective{Kind: KindProgress, Progress: "ss_ticket_acquired"}
	if err := adapter.Validate(o, Observation{GameID: yellowprofile.GameID, PartyCount: 1}); err == nil {
		t.Fatal("a story goal no Yellow controller owns validated")
	}
	result, err := executeYellowOwned(nil, nil, o)
	if !errors.Is(err, errYellowControllerUnavailable) || result.Outcome != OutcomeBlocked {
		t.Fatalf("%s: outcome=%q err=%v, want blocked controller-unavailable", o, result.Outcome, err)
	}
	failure := adapter.NormalizeFailure(game.FailurePhaseExecution, err, Observation{})
	if failure.Class != game.FailureClassBlocked || failure.Recoverable {
		t.Fatalf("%s: failure = %+v, want non-recoverable block", o, failure)
	}
}

func TestYellowSharedObjectivesValidateThroughGen1Engine(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	if adapter.gen1 == nil || adapter.gen1.gameID != yellowprofile.GameID {
		t.Fatalf("Yellow adapter's Gen-I engine = %+v, want one bound to Yellow", adapter.gen1)
	}
	obs := Observation{GameID: yellowprofile.GameID, PartyCount: 1}
	if err := adapter.Validate(Objective{Kind: KindGoTo, Place: "no such place"}, obs); err == nil {
		t.Fatal("unknown destination validated: GoTo is not reaching the shared Gen-I validator")
	}
	if err := adapter.Validate(Objective{Kind: KindGoTo, Place: "viridian city"}, obs); err != nil {
		t.Fatalf("shared GoTo rejected on Yellow: %v", err)
	}
	if err := adapter.Validate(Objective{Kind: KindStarter, Species: "charmander"}, obs); err == nil {
		t.Fatal("Red starter accepted on Yellow")
	}
}

func TestYellowCatalogUsesYellowMapVocabulary(t *testing.T) {
	catalog := yellowObjectiveCatalog(Observation{GameID: yellowprofile.GameID, PartyCount: 1})
	if len(catalog.Starters) != 0 {
		t.Fatalf("starters offered after the Pikachu opening: %+v", catalog.Starters)
	}
	if len(catalog.ChallengeProfiles) != 0 {
		t.Fatalf("Red story challenges offered on Yellow: %+v", catalog.ChallengeProfiles)
	}
	found := false
	for _, d := range catalog.Destinations {
		if d.Place == "viridian city" {
			found = true
			if d.Location != yellowLocationID(yellowprofile.GameID, 0x01) {
				t.Fatalf("viridian city location = %q, want Yellow topology id", d.Location)
			}
		}
	}
	if !found {
		t.Fatal("shared Gen-I destinations missing from the Yellow catalog")
	}
}
