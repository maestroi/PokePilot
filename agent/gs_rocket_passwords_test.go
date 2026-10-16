package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSRocketB3FProgressionNeverSkipsPasswordsOrExecutive(t *testing.T) {
	for _, id := range []string{string(gsprofile.GoldGameID), string(gsprofile.SilverGameID)} {
		adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
		if id == string(gsprofile.SilverGameID) {
			adapter = newGSObjectiveAdapter(nil, nil, gsprofile.SilverGameID)
		}
		obs := Observation{GameID: adapter.gameID, PartyCount: 1,
			Story: ProgressState{
				{ID: gsprofile.ProgressStarterReceived, Complete: true},
				{ID: gsprofile.ProgressMineralBadgeEarned, Complete: true},
				{ID: gsprofile.ProgressRedGyaradosDefeated, Complete: true},
				{ID: gsprofile.ProgressLanceHelpAccepted, Complete: true},
				{ID: gsprofile.ProgressMahoganyStairsOpened, Complete: true},
				{ID: gsprofile.ProgressRocketLanceHealing, Complete: true},
			},
		}
		for _, next := range []ProgressID{
			gsprofile.ProgressRocketPasswordsLearned,
			gsprofile.ProgressRocketOfficeOpened,
			gsprofile.ProgressRocketExecutiveBeaten,
			gsprofile.ProgressRocketMurkrowPassword,
		} {
			goals := adapter.ProgressionObjectives(obs)
			if len(goals) != 1 || goals[0].Progress != next {
				t.Fatalf("%s story returned %+v, want %s", id, goals, next)
			}
			if !adapter.SupportsObjective(goals[0]) {
				t.Fatalf("%s objective %s is not executable", id, next)
			}
			obs.Story = append(obs.Story, ProgressFact{ID: next, Complete: true})
		}
		if goals := adapter.ProgressionObjectives(obs); len(goals) != 0 {
			t.Fatalf("%s may not proceed into unfinished B2F transmitter: %+v", id, goals)
		}
	}
}
