package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSRocketB2FObjectivesRequireRealRetailBoundaries(t *testing.T) {
	if gsRocketTransmitterDoorX != 14 || gsRocketTransmitterDoorY != 12 ||
		gsRocketExecutiveTriggerX != 14 || gsRocketExecutiveTriggerY != 11 {
		t.Fatal("B2F door / executive trigger diverged from TeamRocketBaseB2F.asm")
	}
	b2f, err := gsOpeningMapID("TEAM_ROCKET_BASE_B2F")
	if err != nil || !gsSecondBadgeOwnedMap(b2f) {
		t.Fatalf("B2F native map missing/unsupported: %#x: %v", b2f, err)
	}
}

func TestGSRocketB2FGoldAndSilverSequenceStopsBeforeUnqualifiedPryce(t *testing.T) {
	for _, gameID := range []string{string(gsprofile.GoldGameID), string(gsprofile.SilverGameID)} {
		id := gsprofile.GoldGameID
		if gameID == string(gsprofile.SilverGameID) {
			id = gsprofile.SilverGameID
		}
		adapter := newGSObjectiveAdapter(nil, nil, id)
		obs := Observation{GameID: id, PartyCount: 1, Story: ProgressState{
			{ID: gsprofile.ProgressStarterReceived, Complete: true},
			{ID: gsprofile.ProgressMineralBadgeEarned, Complete: true},
			{ID: gsprofile.ProgressRedGyaradosDefeated, Complete: true},
			{ID: gsprofile.ProgressLanceHelpAccepted, Complete: true},
			{ID: gsprofile.ProgressMahoganyStairsOpened, Complete: true},
			{ID: gsprofile.ProgressRocketLanceHealing, Complete: true},
			{ID: gsprofile.ProgressRocketPasswordsLearned, Complete: true},
			{ID: gsprofile.ProgressRocketOfficeOpened, Complete: true},
			{ID: gsprofile.ProgressRocketExecutiveBeaten, Complete: true},
			{ID: gsprofile.ProgressRocketMurkrowPassword, Complete: true},
		}}
		// Reach B2F from the same prerequisite chain the live planner uses;
		// a fixture with only late event flags still needs earlier stages.
		reached := false
		for step := 0; step < 40; step++ {
			goals := adapter.ProgressionObjectives(obs)
			if len(goals) != 1 {
				t.Fatalf("%s no prerequisite at step %d: %+v", gameID, step, goals)
			}
			if goals[0].Progress == gsprofile.ProgressRocketTransmitterDoor {
				reached = true
				break
			}
			obs.Story = append(obs.Story, ProgressFact{ID: goals[0].Progress, Complete: true})
		}
		if !reached {
			t.Fatalf("%s did not reach transmitter door through Johto prerequisites", gameID)
		}
		for _, next := range []ProgressID{
			gsprofile.ProgressRocketTransmitterDoor,
			gsprofile.ProgressRocketExecutiveF,
			gsprofile.ProgressRocketHideoutCleared,
		} {
			goals := adapter.ProgressionObjectives(obs)
			if len(goals) != 1 || goals[0].Progress != next || !adapter.SupportsObjective(goals[0]) {
				t.Fatalf("%s B2F offer = %+v, want executable %s", gameID, goals, next)
			}
			obs.Story = append(obs.Story, ProgressFact{ID: next, Complete: true})
		}
		if goals := adapter.ProgressionObjectives(obs); len(goals) != 0 {
			t.Fatalf("%s must not offer unqualified Pryce/Glacier completion: %+v", gameID, goals)
		}
		if gsSupportedProgress(gsprofile.ProgressGlacierBadgeEarned) ||
			gsSupportedProgress(gsprofile.ProgressRisingBadgeEarned) {
			t.Fatalf("%s must not unlock badge 7/8 without ROM qualification", gameID)
		}
	}
}
