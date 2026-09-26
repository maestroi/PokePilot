package agent

import (
	"testing"

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

func TestYellowProgressionStopsAfterLabRivalBoundary(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
		},
	}
	if got := a.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("completed opening progression=%v, want no later story objective in this slice", got)
	}
}

func TestYellowProgressionKnownIsBoundedToImplementedSlice(t *testing.T) {
	if !yellowProgressionKnown(yellowprofile.ProgressYellowLabRivalResolved) {
		t.Fatal("lab rival progression must be executable")
	}
	if yellowProgressionKnown("yellow_future_story_gate") {
		t.Fatal("unimplemented Yellow progression must fail closed")
	}
}
