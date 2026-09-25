package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/game"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowstory "github.com/maestroi/pokepilot/yellow/story"
)

type yellowZeroMemory struct{}

func (yellowZeroMemory) Peek8(uint16) byte { return 0 }
func (yellowZeroMemory) PeekInto(_ uint16, dst []byte) {
	for i := range dst {
		dst[i] = 0
	}
}

// Every registered Yellow story goal must have a same-ID verifier in the
// Yellow story projection; otherwise the runtime could never prove it.
func TestYellowProgressionRegistryHasVerifiers(t *testing.T) {
	obs, err := yellowprofile.New().DecodeObservation(yellowZeroMemory{}, nil)
	if err != nil {
		t.Fatalf("DecodeObservation: %v", err)
	}
	if len(yellowProgressionExecutors) == 0 {
		t.Fatal("no Yellow story goals registered")
	}
	for id := range yellowProgressionExecutors {
		if _, ok := obs.Story.Lookup(id); !ok {
			t.Errorf("Yellow progression %q has no projected verifier", id)
		}
	}
}

func TestYellowValidatesOwnedStoryVocabulary(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	obs := Observation{GameID: yellowprofile.GameID}
	for _, o := range []Objective{
		{Kind: KindStarter, Species: "pikachu"},
		{Kind: KindStarter},
		{Kind: KindProgress, Progress: yellowprofile.ProgressYellowStarterReceived},
		{Kind: KindProgress, Progress: yellowprofile.ProgressYellowLabRivalResolved},
	} {
		if err := adapter.Validate(o, obs); err != nil {
			t.Errorf("%s rejected: %v", o, err)
		}
	}
	for _, o := range []Objective{
		{Kind: KindStarter, Species: "bulbasaur"},
		{Kind: KindProgress, Progress: yellowprofile.ProgressYellowMtMoonJessieJamesDefeated},
	} {
		if err := adapter.Validate(o, obs); err == nil {
			t.Errorf("%s validated", o)
		}
	}
}

func yellowStory(complete ...ProgressID) game.ProgressState {
	done := map[ProgressID]bool{}
	for _, id := range complete {
		done[id] = true
	}
	var story game.ProgressState
	for _, id := range []ProgressID{yellowprofile.ProgressYellowStarterReceived, yellowprofile.ProgressYellowLabRivalResolved} {
		story = append(story, game.ProgressFact{ID: id, Complete: done[id]})
	}
	return story
}

func TestYellowOffersTheInterruptedOpeningHalf(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	if got := adapter.ProgressionObjectives(Observation{Story: yellowStory()}); len(got) != 0 {
		t.Fatalf("fresh game offered story goals %v; the Starter objective owns it", got)
	}
	got := adapter.ProgressionObjectives(Observation{Story: yellowStory(yellowprofile.ProgressYellowStarterReceived)})
	if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != yellowprofile.ProgressYellowLabRivalResolved {
		t.Fatalf("interrupted opening offered %v, want the lab rival goal", got)
	}
	done := Observation{Story: yellowStory(yellowprofile.ProgressYellowStarterReceived, yellowprofile.ProgressYellowLabRivalResolved)}
	if got := adapter.ProgressionObjectives(done); len(got) != 0 {
		t.Fatalf("finished opening offered %v", got)
	}
}

func stableYellowObservation() Observation {
	return Observation{GameID: yellowprofile.GameID, Controllable: true, Location: "oaks lab", MapName: "OAKS_LAB"}
}

func TestYellowStarterPostconditionNeedsPikachuAndOpeningFacts(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	o := Objective{Kind: KindStarter, Species: "pikachu"}

	final := stableYellowObservation()
	final.Party = []PartyMon{{Species: "pikachu", Level: 5}}
	final.Story = yellowStory(yellowprofile.ProgressYellowStarterReceived, yellowprofile.ProgressYellowLabRivalResolved)
	if err := adapter.VerifyPostcondition(o, Observation{}, final, ObjectiveResult{}); err != nil {
		t.Fatalf("complete opening rejected: %v", err)
	}

	unresolved := final
	unresolved.Story = yellowStory(yellowprofile.ProgressYellowStarterReceived)
	if err := adapter.VerifyPostcondition(o, Observation{}, unresolved, ObjectiveResult{}); !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("unresolved lab rival err = %v, want postcondition failed", err)
	}

	noPikachu := final
	noPikachu.Party = []PartyMon{{Species: "eevee", Level: 5}}
	if err := adapter.VerifyPostcondition(o, Observation{}, noPikachu, ObjectiveResult{}); !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("party without Pikachu err = %v, want postcondition failed", err)
	}

	// A bare Starter objective (no species) must not fall back to Red's
	// starter-index naming.
	if err := adapter.VerifyPostcondition(Objective{Kind: KindStarter}, Observation{}, final, ObjectiveResult{}); err != nil {
		t.Fatalf("bare Yellow Starter rejected: %v", err)
	}
}

func TestYellowStoryFailuresAreTyped(t *testing.T) {
	adapter := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	for _, tc := range []struct {
		err   error
		class game.FailureClass
		cause string
	}{
		{yellowstory.ErrOpeningStalled, game.FailureClassControllerUncertain, "yellow_opening_stalled"},
		{yellowstory.ErrOpeningUnexpectedState, game.FailureClassControllerUncertain, "yellow_opening_unexpected_state"},
		{yellowstory.ErrOpeningChoicePrompt, game.FailureClassChoiceRequired, "yellow_opening_choice_prompt"},
	} {
		err := fmt.Errorf("agent: starter: %w", tc.err)
		failure := adapter.NormalizeFailure(game.FailurePhaseExecution, err, Observation{})
		if failure.Class != tc.class || failure.Cause != tc.cause || failure.Recoverable {
			t.Errorf("%v -> %+v, want class %s cause %s non-recoverable", tc.err, failure, tc.class, tc.cause)
		}
	}
}

func TestYellowOfferRoutesThroughYellowProgressionPlanner(t *testing.T) {
	obs := Observation{
		GameID: yellowprofile.GameID, PartyCount: 1, Controllable: true,
		Party: []PartyMon{{Species: "pikachu", Level: 5}},
		Story: yellowStory(yellowprofile.ProgressYellowStarterReceived),
	}
	offer := offerWithTMHMEvidence(nil, nil, obs, NewKnowledge(nil))
	for _, o := range offer.Candidates {
		if o.Kind == KindProgress && o.Progress == yellowprofile.ProgressYellowLabRivalResolved {
			return
		}
	}
	t.Fatalf("Yellow offer %v is missing the interrupted opening's lab rival goal", offer.Candidates)
}
