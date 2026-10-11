package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// TestOfferKeepsRecoveryTrainWhenTrainItselfRecordedCombatLoss guards the
// cancel-resume dead end from run-36loaokprjdtw2j8om666lswgf
// (triage:e419e12a9a0bc13a): a wild wipe during combat-recovery grind was
// recorded as combat_defeat on "train the lead to level N". The portable
// combat-loss gate then withheld both the Hive fight and that same train,
// emptying the menu into "nothing is possible from here" even though a grind
// corridor still exists.
func TestOfferKeepsRecoveryTrainWhenTrainItselfRecordedCombatLoss(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		obs := Observation{
			GameID: id, PartyCount: 2, Controllable: true,
			Party: []PartyMon{
				{Species: "meganium", Level: 32, HP: 112, MaxHP: 112},
				{Species: "egg", Level: 5, IsEgg: true, HP: 0, MaxHP: 19},
			},
			Story: ProgressState{
				{ID: gsprofile.ProgressStarterReceived, Complete: true},
				{ID: gsprofile.ProgressMysteryEggReturned, Complete: true},
				{ID: gsprofile.ProgressSproutTowerCleared, Complete: true},
				{ID: gsprofile.ProgressZephyrBadgeEarned, Complete: true},
				{ID: gsprofile.ProgressTogepiEggReceived, Complete: true},
				{ID: gsprofile.ProgressSlowpokeWellCleared, Complete: true},
			},
		}
		hive := Objective{Kind: KindProgress, Progress: gsprofile.ProgressHiveBadgeEarned}
		train := Objective{Kind: KindTrain, Slot: 0, Level: 33}
		known := NewKnowledge(nil)
		known.Failures[combatLossFailureKey(hive)] = Failure{
			Objective: hive.String(), Times: 1,
			ReadinessBaseline: 110, ReadinessTarget: 140,
		}
		// Poisoned pre-fix checkpoint shape: train wipe stored under combat_loss.
		known.Failures[combatLossFailureKey(train)] = Failure{
			Objective: train.String(), Times: 1,
			Last: "required battle gen2:second-badge-route:0320 ended with outcome 1",
		}
		obs.Failures = known.FailureList()

		if !adapter.CombatRecoveryAvailable(obs) {
			t.Fatalf("%s: test assumption wrong: recovery corridor unavailable", id)
		}
		offer := OfferWithProgressionEvidence(obs, known, adapter)
		var gotTrain *Objective
		for i := range offer.Candidates {
			if offer.Candidates[i].Kind == KindProgress && offer.Candidates[i].Progress == hive.Progress {
				t.Fatalf("%s: combat-lost Hive fight re-offered: %+v", id, offer.Candidates)
			}
			if offer.Candidates[i].Kind == KindTrain && offer.Candidates[i].Slot == 0 {
				gotTrain = &offer.Candidates[i]
			}
		}
		// A fresh rung may be offered (L34 here): the important invariant is that
		// preparation remains executable even when an older train key still carries
		// a poisoned combat_loss marker.
		if gotTrain == nil || gotTrain.Level <= 32 {
			t.Fatalf("%s: want executable recovery train above L32 after poisoned train combat_loss; candidates=%v", id, offer.Candidates)
		}
	}
}

func TestTrainCombatDefeatDoesNotEnterCombatLossMode(t *testing.T) {
	train := Objective{Kind: KindTrain, Slot: 0, Level: 33}
	err := skill.RequireBattleWin("gen2:second-badge-route:0320", game.BattleLost)
	if _, ok := combatLossFailureName(train, err); ok {
		t.Fatalf("train blackout entered combat-loss mode: %v", err)
	}
	if !errors.Is(err, skill.ErrBlackedOut) {
		t.Fatalf("test assumption wrong: want blackout sentinel, got %v", err)
	}

	known := NewKnowledge(nil)
	known.FailedResult(ObjectiveResult{
		Objective: train,
		Outcome:   OutcomeBlocked,
		Cause:     FailureCauseID(failureCauseCombatDefeat),
		Battle:    &BattleEvidence{Result: "lost"},
		Final: Observation{
			PartyCount: 1,
			Party:      []PartyMon{{Species: "meganium", Level: 32, HP: 112, MaxHP: 112}},
		},
	}, err)
	if _, ok := known.Failures[combatLossFailureKey(train)]; ok {
		t.Fatalf("FailedResult stamped combat_loss on train: %+v", known.Failures)
	}
	if _, ok := known.Failures[objectiveStorageKey(train)]; !ok {
		t.Fatalf("FailedResult did not keep ordinary train failure: %+v", known.Failures)
	}
}
