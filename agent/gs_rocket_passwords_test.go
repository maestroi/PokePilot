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
		// The production planner is linear: having reached B2F does not
		// magically satisfy all earlier Johto prerequisites in a unit fixture.
		// Simulate the actual prerequisite chain before asserting this slice.
		reached := false
		for step := 0; step < 40; step++ {
			goals := adapter.ProgressionObjectives(obs)
			if len(goals) != 1 {
				t.Fatalf("%s has no prerequisite stage at step %d: %+v", id, step, goals)
			}
			if goals[0].Progress == gsprofile.ProgressRocketPasswordsLearned {
				reached = true
				break
			}
			obs.Story = append(obs.Story, ProgressFact{ID: goals[0].Progress, Complete: true})
		}
		if !reached {
			t.Fatalf("%s did not reach B3F through the story prerequisites", id)
		}
		for _, next := range []ProgressID{
			gsprofile.ProgressRocketPasswordsLearned,
			gsprofile.ProgressRocketOfficeOpened,
			gsprofile.ProgressRocketExecutiveBeaten,
			gsprofile.ProgressRocketMurkrowPassword,
			gsprofile.ProgressRocketTransmitterDoorOpened,
			gsprofile.ProgressRocketExecutiveFBeaten,
			gsprofile.ProgressRocketHideoutCleared,
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
		if goals := adapter.ProgressionObjectives(obs); len(goals) != 1 ||
			goals[0].Progress != gsprofile.ProgressGlacierBadgeEarned {
			t.Fatalf("%s must offer experimental Pryce only after Rocket clear: %+v", id, goals)
		}
	}
}
