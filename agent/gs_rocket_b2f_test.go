package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSRocketB2FStagesAreSelectableBeforeGlacier(t *testing.T) {
	for _, gameID := range []string{string(gsprofile.GoldGameID), string(gsprofile.SilverGameID)} {
		adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
		if gameID == string(gsprofile.SilverGameID) {
			adapter = newGSObjectiveAdapter(nil, nil, gsprofile.SilverGameID)
		}
		obs := Observation{GameID: adapter.gameID, PartyCount: 1,
			Story: ProgressState{{ID: gsprofile.ProgressStarterReceived, Complete: true}}}
		stages := []ProgressID{
			gsprofile.ProgressRocketTransmitterDoorOpened,
			gsprofile.ProgressRocketExecutiveFBeaten,
			gsprofile.ProgressRocketHideoutCleared,
		}
		got := []ProgressID{}
		seenGlacier := false
		for attempt := 0; attempt < 60; attempt++ {
			goals := adapter.ProgressionObjectives(obs)
			if len(goals) == 0 {
				break
			}
			if len(goals) != 1 {
				t.Fatalf("%s unexpected progression %+v", gameID, goals)
			}
			goal := goals[0]
			if goal.Progress == gsprofile.ProgressGlacierBadgeEarned {
				seenGlacier = true
			}
			if !adapter.SupportsObjective(goal) {
				t.Fatalf("%s unsupported objective %s was offered", gameID, goal.Progress)
			}
			for _, stage := range stages {
				if goal.Progress == stage {
					got = append(got, stage)
				}
			}
			obs.Story = append(obs.Story, ProgressFact{ID: goal.Progress, Complete: true})
		}
		if len(got) != len(stages) {
			t.Fatalf("%s B2F stages missing: got %v, want %v", gameID, got, stages)
		}
		for i := range stages {
			if got[i] != stages[i] {
				t.Fatalf("%s B2F order = %v, want %v", gameID, got, stages)
			}
		}
		if !obs.Story.Has(gsprofile.ProgressRocketHideoutCleared) {
			t.Fatal("did not clear Rocket Hideout")
		}
		if !seenGlacier {
			t.Fatalf("%s experimental B2F run never offered Pryce", gameID)
		}
		if goals := adapter.ProgressionObjectives(obs); len(goals) != 0 {
			t.Fatalf("%s planner invented unimplemented objective beyond Radio Tower: %+v", gameID, goals)
		}
		if !gsSupportedProgress(gsprofile.ProgressGlacierBadgeEarned) {
			t.Fatal("opt-in Glacier stage must be executable")
		}
	}
}
