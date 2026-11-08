package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSExperimentalEightBadgeCampaignContinuesFromSixThroughTower(t *testing.T) {
	afterSix := []ProgressID{
		gsprofile.ProgressRedGyaradosDefeated,
		gsprofile.ProgressLanceHelpAccepted,
		gsprofile.ProgressMahoganyStairsOpened,
		gsprofile.ProgressRocketLanceHealing,
		gsprofile.ProgressRocketPasswordsLearned,
		gsprofile.ProgressRocketOfficeOpened,
		gsprofile.ProgressRocketExecutiveBeaten,
		gsprofile.ProgressRocketMurkrowPassword,
		gsprofile.ProgressRocketTransmitterDoorOpened,
		gsprofile.ProgressRocketExecutiveFBeaten,
		gsprofile.ProgressRocketHideoutCleared,
		gsprofile.ProgressGlacierBadgeEarned,
		gsprofile.ProgressRadioTowerFakeDirectorBeaten,
		gsprofile.ProgressRadioTowerBasementUnlocked,
		gsprofile.ProgressRadioTowerCardKeyReceived,
		gsprofile.ProgressRadioTowerShutterOpened,
		gsprofile.ProgressRadioTowerExecutiveM2Beaten,
		gsprofile.ProgressRadioTowerExecutiveF1Beaten,
		gsprofile.ProgressRadioTowerTakeoverSettled,
		gsprofile.ProgressIcePathBouldersDropped,
		gsprofile.ProgressIcePathWaterfallAcquired,
		gsprofile.ProgressBlackthornGymBouldersDropped,
		gsprofile.ProgressClairDefeated,
		gsprofile.ProgressRisingBadgeEarned,
	}
	for _, id := range []struct {
		name   string
		gameID game.GameID
	}{
		{"gold", gsprofile.GoldGameID},
		{"silver", gsprofile.SilverGameID},
	} {
		t.Run(id.name, func(t *testing.T) {
			adapter := newGSObjectiveAdapter(nil, nil, id.gameID)
			obs := Observation{GameID: id.gameID, PartyCount: 1, Story: ProgressState{}}
			for _, fact := range []ProgressID{
				gsprofile.ProgressStarterReceived,
				gsprofile.ProgressMysteryEggReturned,
				gsprofile.ProgressSproutTowerCleared,
				gsprofile.ProgressZephyrBadgeEarned,
				gsprofile.ProgressTogepiEggReceived,
				gsprofile.ProgressSlowpokeWellCleared,
				gsprofile.ProgressHiveBadgeEarned,
				gsprofile.ProgressAzaleaRivalResolved,
				gsprofile.ProgressFarfetchdHerded,
				gsprofile.ProgressHM01CutAcquired,
				gsprofile.ProgressTM02HeadbuttAcquired,
				gsprofile.ProgressPlainBadgeEarned,
				gsprofile.ProgressSquirtBottleAcquired,
				gsprofile.ProgressSudowoodoCleared,
				gsprofile.ProgressTM08RockSmashAcquired,
				gsprofile.ProgressBurnedTowerCleared,
				gsprofile.ProgressFogBadgeEarned,
				gsprofile.ProgressHM03SurfAcquired,
				gsprofile.ProgressLighthouseSicknessExplained,
				gsprofile.ProgressSecretPotionAcquired,
				gsprofile.ProgressJasmineReturnedToGym,
				gsprofile.ProgressHM04StrengthAcquired,
				gsprofile.ProgressStormBadgeEarned,
				gsprofile.ProgressHM02FlyAcquired,
				gsprofile.ProgressMineralBadgeEarned,
			} {
				obs.Story = append(obs.Story, ProgressFact{ID: fact, Complete: true})
			}
			for _, next := range afterSix {
				goals := adapter.ProgressionObjectives(obs)
				if len(goals) != 1 || goals[0].Progress != next {
					t.Fatalf("after six badges, next=%s got=%+v", next, goals)
				}
				if !adapter.SupportsObjective(goals[0]) {
					t.Fatalf("planner offered unavailable stage %s", next)
				}
				obs.Story = append(obs.Story, ProgressFact{ID: next, Complete: true})
			}
			// Past eight badges the League approach precedes the experimental
			// Elite Four and the event-verified Hall of Fame.
			if goals := adapter.ProgressionObjectives(obs); len(goals) != 1 ||
				goals[0].Progress != gsprofile.ProgressIndigoPlateauReached || !adapter.SupportsObjective(goals[0]) {
				t.Fatalf("completed Rising Badge must offer only the Indigo Plateau approach: %+v", goals)
			}
			obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressIndigoPlateauReached, Complete: true})
			if goals := adapter.ProgressionObjectives(obs); len(goals) != 1 ||
				goals[0].Progress != gsprofile.ProgressLeagueWillDefeated || !adapter.SupportsObjective(goals[0]) {
				t.Fatalf("Indigo Plateau should offer only the Will challenge: %+v", goals)
			}
			league := []ProgressID{
				gsprofile.ProgressLeagueWillDefeated,
				gsprofile.ProgressLeagueKogaDefeated,
				gsprofile.ProgressLeagueBrunoDefeated,
				gsprofile.ProgressLeagueKarenDefeated,
				gsprofile.ProgressLeagueLanceDefeated,
				gsprofile.ProgressLeagueHallOfFameEntered,
			}
			for i, completed := range league {
				obs.Story = append(obs.Story, ProgressFact{ID: completed, Complete: true})
				goals := adapter.ProgressionObjectives(obs)
				if i == len(league)-1 {
					if len(goals) != 1 || goals[0].Progress != gsprofile.ProgressSSTicketReceived ||
						!adapter.SupportsObjective(goals[0]) {
						t.Fatalf("Hall of Fame must offer the post-League S.S. Ticket: %+v", goals)
					}
					continue
				}
				want := league[i+1]
				if len(goals) != 1 || goals[0].Progress != want || !adapter.SupportsObjective(goals[0]) {
					t.Fatalf("after %s expect only %s: %+v", completed, want, goals)
				}
			}
			obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressSSTicketReceived, Complete: true})
			if goals := adapter.ProgressionObjectives(obs); len(goals) != 1 ||
				goals[0].Progress != gsprofile.ProgressKantoFirstLanding || !adapter.SupportsObjective(goals[0]) {
				t.Fatalf("ticket should offer the real first Kanto landing: %+v", goals)
			}
			obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressKantoFirstLanding, Complete: true})
			if goals := adapter.ProgressionObjectives(obs); len(goals) != 1 ||
				goals[0].Progress != gsprofile.ProgressKantoThunderBadgeEarned || !adapter.SupportsObjective(goals[0]) {
				t.Fatalf("Vermilion landing must offer Lt. Surge's Kanto Thunder Badge: %+v", goals)
			}
			obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressKantoThunderBadgeEarned, Complete: true})
			if goals := adapter.ProgressionObjectives(obs); len(goals) != 0 {
				t.Fatalf("Thunder Badge ends the current post-League frontier: %+v", goals)
			}
			if !gsSupportedProgress(gsprofile.ProgressRisingBadgeEarned) {
				t.Fatal("Rising Badge should now have an owned executor")
			}
			// Seven badges must not satisfy an opted-in 8-badge campaign.
			obs.Badges = []string{"zephyr", "hive", "plain", "fog", "mineral", "storm", "glacier"}
			before, recognized, err := PlannerGoalStatus("Earn 8 badges.", obs)
			if err != nil || !recognized || before.Complete || before.Current != 7 || before.Target != 8 {
				t.Fatalf("7/8 must remain incomplete: %+v structured=%v err=%v", before, recognized, err)
			}
			obs.Badges = append(obs.Badges, "rising")
			after, recognized, err := PlannerGoalStatus("Earn 8 badges.", obs)
			if err != nil || !recognized || !after.Complete || after.Current != 8 {
				t.Fatalf("8 real badges must satisfy the run goal: %+v structured=%v err=%v", after, recognized, err)
			}
		})
	}
}
