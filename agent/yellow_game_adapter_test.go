package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
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

func TestYellowOpeningObjectivesUseYellowControllerNotRedScripts(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	for _, o := range []Objective{
		{Kind: KindStarter, Species: "pikachu"},
		{Kind: KindProgress, Progress: yellowprofile.ProgressYellowLabRivalResolved},
	} {
		_, err := adapter.ExecuteOwned(o)
		if err == nil {
			t.Fatalf("%s: nil emulator unexpectedly executed", o)
		}
		if errors.Is(err, errYellowControllerUnavailable) {
			t.Fatalf("%s: opening still reports controller unavailable: %v", o, err)
		}
	}

	unsupported := Objective{Kind: KindProgress, Progress: "oak-parcel"}
	if err := adapter.Validate(unsupported, Observation{GameID: yellowprofile.GameID}); err == nil {
		t.Fatalf("%s: unimplemented Yellow story goal validated", unsupported)
	}
}

func TestYellowUnimplementedProgressionStillNormalizesAsTypedBlock(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	o := Objective{Kind: KindProgress, Progress: "yellow_future_story_gate"}
	result, err := adapter.ExecuteOwned(o)
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

func TestYellowDefaultStarterObjectiveIsScriptedPikachu(t *testing.T) {
	obs := Observation{GameID: yellowprofile.GameID, PartyCount: 0}
	got, ok := DefaultStarterObjective(obs)
	if !ok || got.Kind != KindStarter || got.Species != "pikachu" {
		t.Fatalf("DefaultStarterObjective(Yellow) = %+v,%v, want Pikachu starter", got, ok)
	}
}

func TestYellowEarlySharedProgressionContinuesAfterOpening(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
		},
	}
	got := adapter.ProgressionObjectives(obs)
	if !hasProgressObjective(got, gen1.ProgressPokedexAcquired) {
		t.Fatalf("post-opening Yellow objectives = %v, want Pokedex acquisition", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressPokedexAcquired, Complete: true})
	got = adapter.ProgressionObjectives(obs)
	if !hasProgressObjective(got, gen1.ProgressBoulderBadge) {
		t.Fatalf("post-Pokedex Yellow objectives = %v, want Boulder Badge", got)
	}
}


func TestYellowOpeningFailuresNormalizeWithoutUnknownFailure(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	tests := []struct {
		err   error
		class game.FailureClass
		cause string
	}{
		{errYellowOpeningStalled, game.FailureClassControllerUncertain, "yellow_opening_stalled"},
		{errYellowOpeningUnexpectedState, game.FailureClassControllerUncertain, "yellow_opening_unexpected_state"},
		{errYellowOpeningChoiceRequired, game.FailureClassChoiceRequired, "yellow_opening_choice_required"},
	}
	for _, tc := range tests {
		got := adapter.NormalizeFailure(game.FailurePhaseExecution, tc.err, Observation{})
		if got.Class != tc.class || got.Cause != tc.cause {
			t.Fatalf("NormalizeFailure(%v)=%+v, want class=%q cause=%q", tc.err, got, tc.class, tc.cause)
		}
	}
}

func TestYellowStarterPostconditionRequiresWholeOpening(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	o := Objective{Kind: KindStarter, Species: "pikachu"}
	base := Observation{
		GameID:       yellowprofile.GameID,
		Controllable: true,
		Party:        []PartyMon{{Species: "pikachu"}},
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: false},
		},
	}
	if err := adapter.VerifyPostcondition(o, Observation{}, base, ObjectiveResult{Objective: o}); err == nil {
		t.Fatal("starter postcondition accepted Pikachu before the lab rival resolved")
	}
	base.Story = ProgressState{
		{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
		{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
	}
	if err := adapter.VerifyPostcondition(o, Observation{}, base, ObjectiveResult{Objective: o}); err != nil {
		t.Fatalf("completed Yellow opening rejected: %v", err)
	}
}
