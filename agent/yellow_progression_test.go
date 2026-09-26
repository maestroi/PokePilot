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

func TestYellowProgressionContinuesToPokedexAfterLabRival(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
		},
	}
	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressPokedexAcquired {
		t.Fatalf("completed opening progression=%v, want Pokedex acquisition", got)
	}
}

func TestYellowProgressionKnownIsBoundedToImplementedSlice(t *testing.T) {
	for _, id := range []ProgressID{
		yellowprofile.ProgressYellowLabRivalResolved,
		gen1.ProgressPokedexAcquired,
		gen1.ProgressBoulderBadge,
		gen1.ProgressMtMoonFossilAcquired,
		yellowprofile.ProgressYellowMtMoonExitResolved,
	} {
		if !yellowProgressionKnown(id) {
			t.Fatalf("%q progression must be executable", id)
		}
	}
	if yellowProgressionKnown("yellow_future_story_gate") {
		t.Fatal("unimplemented Yellow progression must fail closed")
	}
}

func TestYellowProgressionInsertsJessieJamesAfterMtMoonFossil(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID: yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
			{ID: gen1.ProgressPokedexAcquired, Complete: true},
			{ID: gen1.ProgressBoulderBadge, Complete: true},
		},
	}
	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressMtMoonFossilAcquired {
		t.Fatalf("post-Brock progression=%v, want Mt Moon fossil", got)
	}
	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressMtMoonFossilAcquired, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != yellowprofile.ProgressYellowMtMoonExitResolved {
		t.Fatalf("post-fossil progression=%v, want Yellow Jessie/James exit", got)
	}
	obs.Story = append(obs.Story, ProgressFact{ID: yellowprofile.ProgressYellowMtMoonExitResolved, Complete: true})
	if got = a.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("Mt Moon slice should stop after Yellow exit until next controller lands: %v", got)
	}
}
