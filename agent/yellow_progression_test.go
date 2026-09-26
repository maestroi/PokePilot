package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestYellowProgressionLeavesFreshOpeningToStarterProvider(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{GameID: yellowprofile.GameID}
	if got := a.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("fresh opening progression=%v, want starter provider ownership", got)
	}
}

func TestYellowProgressionResumesOpeningAfterStarterReceipt(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
		},
	}
	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != yellowprofile.ProgressYellowLabRivalResolved {
		t.Fatalf("opening recovery progression=%v, want lab rival resolution", got)
	}
}

func TestYellowProgressionReusesSharedPokedexAndBrockOrder(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	base := ProgressState{
		{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
		{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
	}

	got := a.ProgressionObjectives(Observation{GameID: yellowprofile.GameID, PartyCount: 1, Story: base})
	if len(got) != 1 || got[0].Progress != gen1.ProgressPokedexAcquired {
		t.Fatalf("after Yellow opening = %v, want shared Pokedex progression", got)
	}

	withDex := append(append(ProgressState{}, base...), ProgressFact{ID: gen1.ProgressPokedexAcquired, Complete: true})
	got = a.ProgressionObjectives(Observation{GameID: yellowprofile.GameID, PartyCount: 1, Story: withDex})
	if len(got) != 1 || got[0].Progress != gen1.ProgressBoulderBadge {
		t.Fatalf("after Pokedex = %v, want shared Boulder progression", got)
	}

	done := append(append(ProgressState{}, withDex...), ProgressFact{ID: gen1.ProgressBoulderBadge, Complete: true})
	if got := a.ProgressionObjectives(Observation{GameID: yellowprofile.GameID, PartyCount: 1, Story: done}); len(got) != 0 {
		t.Fatalf("after Boulder = %v, want this slice complete", got)
	}
}

func TestYellowProgressionKnownIsBoundedToImplementedSlice(t *testing.T) {
	for _, id := range []ProgressID{
		yellowprofile.ProgressYellowLabRivalResolved,
		gen1.ProgressPokedexAcquired,
		gen1.ProgressBoulderBadge,
	} {
		if !yellowProgressionKnown(id) {
			t.Fatalf("implemented Yellow progression %q rejected", id)
		}
	}
	if yellowProgressionKnown("yellow_future_story_gate") {
		t.Fatal("unimplemented Yellow progression must fail closed")
	}
}
