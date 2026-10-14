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

// TestOfferReleasesSatisfiedCombatReadinessOnResume guards the cancel-resume
// dead end from run-ldnbzicnkw7cyoz4h3mins9c (triage:e419e12a9a0bc13a): a
// combat-preparation grind was cancelled after the lead already reached the
// loss's readiness target, so the loss was never promoted to retry (that
// promotion only runs after an objective completes). On resume the target is
// met, so no training rung is offered, but the fight is still withheld and the
// sealed-topology escape is gated on owesCombatReadiness (false when the target
// is met). The menu empties into "nothing is possible from here". A satisfied
// readiness target must release the gate at offer time.
func TestOfferReleasesSatisfiedCombatReadinessOnResume(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		const leadLevel = 49
		obs := Observation{
			GameID: id, PartyCount: 1, Controllable: true,
			Party: []PartyMon{
				{Species: "meganium", Level: leadLevel, HP: 112, MaxHP: 112},
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
		known := NewKnowledge(nil)
		// A quantified loss whose target the L49 lead already meets: readiness
		// 4*49 = 196. The satisfying grind was cancelled before it completed, so
		// the loss is still in combat-loss mode (never promoted to retry).
		known.Failures[combatLossFailureKey(hive)] = Failure{
			Objective: hive.String(), Times: 1,
			ReadinessBaseline: 100, ReadinessTarget: 196,
		}
		obs.Failures = known.FailureList()

		// Test assumptions: the target is met and no training rung is offered.
		if owesCombatReadiness(known, obs) {
			t.Fatalf("%s: test assumption wrong: readiness target should be met", id)
		}
		if target := gsCombatRecoveryLeadTarget(obs, known.Failures[combatLossFailureKey(hive)]); target > 0 {
			t.Fatalf("%s: test assumption wrong: want no training rung at a met target, got %d", id, target)
		}

		offer := OfferWithProgressionEvidence(obs, known, adapter)
		var gotRetry *Objective
		for i := range offer.Candidates {
			if offer.Candidates[i].Kind == KindProgress && offer.Candidates[i].Progress == hive.Progress {
				gotRetry = &offer.Candidates[i]
			}
		}
		if gotRetry == nil {
			t.Fatalf("%s: want the satisfied Hive fight re-offered as a retry (cancel-resume dead end); candidates=%v", id, offer.Candidates)
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
