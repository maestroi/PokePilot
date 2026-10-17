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
			if goals := adapter.ProgressionObjectives(obs); len(goals) != 0 {
				t.Fatalf("unimplemented Ice Path/Clair must not be offered: %+v", goals)
			}
			if gsSupportedProgress(gsprofile.ProgressRisingBadgeEarned) {
				t.Fatal("Rising Badge does not have an executor yet")
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
