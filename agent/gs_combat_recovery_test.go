package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

func gsMortyRecoveryObservation(id game.GameID) Observation {
	return Observation{
		GameID: id, PartyCount: 1, Controllable: true,
		Party: []PartyMon{{Species: "meganium", Level: 34, HP: 109, MaxHP: 109}},
		Story: ProgressState{
			{ID: gsprofile.ProgressStarterReceived, Complete: true},
			{ID: gsprofile.ProgressMysteryEggReturned, Complete: true},
			{ID: gsprofile.ProgressSproutTowerCleared, Complete: true},
			{ID: gsprofile.ProgressZephyrBadgeEarned, Complete: true},
			{ID: gsprofile.ProgressTogepiEggReceived, Complete: true},
			{ID: gsprofile.ProgressSlowpokeWellCleared, Complete: true},
			{ID: gsprofile.ProgressHiveBadgeEarned, Complete: true},
			{ID: gsprofile.ProgressAzaleaRivalResolved, Complete: true},
			{ID: gsprofile.ProgressFarfetchdHerded, Complete: true},
			{ID: gsprofile.ProgressHM01CutAcquired, Complete: true},
			{ID: gsprofile.ProgressTM02HeadbuttAcquired, Complete: true},
			{ID: gsprofile.ProgressPlainBadgeEarned, Complete: true},
			{ID: gsprofile.ProgressSquirtBottleAcquired, Complete: true},
			{ID: gsprofile.ProgressSudowoodoCleared, Complete: true},
			{ID: gsprofile.ProgressTM08RockSmashAcquired, Complete: true},
			{ID: gsprofile.ProgressBurnedTowerCleared, Complete: true},
		},
	}
}

func TestGSCombatLossOffersExecutableTrainWithoutRetryingBlockedMorty(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		obs := gsMortyRecoveryObservation(id)
		fight := Objective{Kind: KindProgress, Progress: gsprofile.ProgressFogBadgeEarned}
		known := NewKnowledge(nil)
		known.Failures[combatLossFailureKey(fight)] = Failure{
			Objective: fight.String(), Times: 1,
			ReadinessBaseline: 34 * 4, ReadinessTarget: 39 * 4,
		}
		obs.Failures = known.FailureList()
		out := OfferWithProgressionEvidence(obs, known, adapter).Candidates
		var train *Objective
		for i := range out {
			if out[i].Kind == KindProgress && out[i].Progress == fight.Progress {
				t.Fatalf("%s: lost Morty incorrectly offered again: %+v", id, out)
			}
			if out[i].Kind == KindTrain && out[i].Slot == 0 {
				train = &out[i]
			}
		}
		if train == nil || train.Level != 36 {
			t.Fatalf("%s: want executable lead training to L36 after loss: %+v", id, out)
		}
		if err := adapter.Validate(*train, obs); err != nil {
			t.Fatalf("%s: recovery train rejected by adapter: %v", id, err)
		}
	}
}

// TestGSFirstBadgeLossPromotesToRetryWithoutCorridor guards the pre-Slowpoke-Well
// boundary: a first-badge (Violet Gym) loss records a combat-loss gate on the
// Zephyr objective, but Gen2 owns no proven wild-grind corridor there, so no
// training can be offered. The generic gate must promote the loss to retry and
// re-offer the fight after the game's own heal, instead of emptying the menu
// and dying with "nothing is possible from here"
// (run-36loaokprjdtw2j8om666lswgf).
func TestGSFirstBadgeLossPromotesToRetryWithoutCorridor(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		obs := Observation{
			GameID: id, PartyCount: 1, Controllable: true,
			Party: []PartyMon{{Species: "chikorita", Level: 14, HP: 50, MaxHP: 50}},
			Story: ProgressState{
				{ID: gsprofile.ProgressStarterReceived, Complete: true},
				{ID: gsprofile.ProgressMysteryEggReturned, Complete: true},
				{ID: gsprofile.ProgressSproutTowerCleared, Complete: true},
			},
		}
		fight := Objective{Kind: KindProgress, Progress: gsprofile.ProgressZephyrBadgeEarned}
		known := NewKnowledge(nil)
		known.Failures[combatLossFailureKey(fight)] = Failure{
			Objective: fight.String(), Times: 1,
			ReadinessBaseline: 14 * 4, ReadinessTarget: 76,
		}
		obs.Failures = known.FailureList()

		if adapter.CombatRecoveryAvailable(obs) {
			t.Fatalf("%s: first-badge boundary reported a recovery corridor", id)
		}

		out := OfferWithProgressionEvidence(obs, known, adapter).Candidates
		reoffered := false
		for i := range out {
			if out[i].Kind == KindProgress && out[i].Progress == fight.Progress {
				reoffered = true
			}
		}
		if !reoffered {
			t.Fatalf("%s: first-badge loss left an empty menu, want the fight re-offered: %+v", id, out)
		}
		if _, ok := known.Failures[combatLossFailureKey(fight)]; ok {
			t.Fatalf("%s: combat-loss gate not promoted to retry: %+v", id, known.Failures)
		}
	}
}

func TestGSCombatRecoveryTargetAndProvenHabitat(t *testing.T) {
	obs := gsMortyRecoveryObservation(gsprofile.GoldGameID)
	if target := gsCombatRecoveryLeadTarget(obs, Failure{Times: 1, ReadinessTarget: 34*4 + 20}); target != 36 {
		t.Fatalf("quantified solo recovery target=%d, want 36", target)
	}
	if target := gsCombatRecoveryLeadTarget(obs, Failure{Times: 1}); target != 36 {
		t.Fatalf("legacy recovery target=%d, want 36", target)
	}
	if target := gsCombatRecoveryLeadTarget(obs, Failure{Times: 1, ReadinessTarget: 34 * 4}); target != 0 {
		t.Fatalf("recovery target already satisfied = %d, want no extra grind", target)
	}
	if name, x, y, err := gsRecoveryGrindSite(obs); err != nil || name != "ROUTE_37" || x != 15 || y != 14 {
		t.Fatalf("post-Sudowoodo grind site = %s (%d,%d), %v", name, x, y, err)
	}
	obs.Story = ProgressState{{ID: gsprofile.ProgressSlowpokeWellCleared, Complete: true}}
	if name, x, y, err := gsRecoveryGrindSite(obs); err != nil || name != "SLOWPOKE_WELL_B1F" || x != 5 || y != 3 {
		t.Fatalf("Bugsy grind site = %s (%d,%d), %v", name, x, y, err)
	}
	obs.Story = nil
	if _, _, _, err := gsRecoveryGrindSite(obs); !errors.Is(err, errGSControllerUnavailable) {
		t.Fatalf("unsupported early-Johto grind site = %v, want unimplemented", err)
	}
}

// TestPartyCombatReadinessIgnoresEggs pins the quantified combat-prep invariant
// from run-36loaokprjdtw2j8om666lswgf: an egg's stored level must not pad a
// lone lead up to a readiness target and release a Bugsy retry early.
func TestPartyCombatReadinessIgnoresEggs(t *testing.T) {
	obs := Observation{
		Party: []PartyMon{
			{Species: "bayleef", Level: 29, HP: 89, MaxHP: 89},
			{Species: "egg", Level: 5, IsEgg: true},
		},
	}
	if got := partyCombatReadiness(obs); got != 29*4 {
		t.Fatalf("readiness=%d, want %d from the battler only", got, 29*4)
	}
	fight := Objective{Kind: KindProgress, Progress: gsprofile.ProgressHiveBadgeEarned}
	failure := Failure{Objective: fight.String(), Times: 1}
	stampCombatPreparation(&failure, obs)
	if failure.ReadinessBaseline != 29*4 {
		t.Fatalf("baseline=%d, want %d", failure.ReadinessBaseline, 29*4)
	}
	// Thin real party (one battler) asks for five levels: 3 + 2 thin-party margin.
	if want := 29*4 + 5*4; failure.ReadinessTarget != want {
		t.Fatalf("target=%d, want %d", failure.ReadinessTarget, want)
	}
}

func TestGSCombatRecoveryDoesNotCreateSpuriousTraining(t *testing.T) {
	obs := gsMortyRecoveryObservation(gsprofile.GoldGameID)
	fight := Objective{Kind: KindProgress, Progress: gsprofile.ProgressFogBadgeEarned}
	if got := gsProgressionCombatRecovery(obs, fight); len(got) != 0 {
		t.Fatalf("training without combat defeat: %+v", got)
	}
	obs.Failures = []Failure{{Objective: fight.String(), Times: 1, ReadinessBaseline: 136, ReadinessTarget: 156}}
	if got := gsProgressionCombatRecovery(obs, fight); len(got) != 1 || got[0].Level != 36 {
		t.Fatalf("expected L36 recovery after confirmed loss: %+v", got)
	}
	if got := gsProgressionCombatRecovery(obs, Objective{Kind: KindProgress, Progress: gsprofile.ProgressSecretPotionAcquired}); len(got) > 0 {
		t.Fatalf("unrelated quest inherited Morty training: %+v", got)
	}
	obs.Party[0].IsEgg = true
	if got := gsProgressionCombatRecovery(obs, fight); len(got) > 0 {
		t.Fatalf("egg lead offered train objective: %+v", got)
	}
	obs.Party[0].IsEgg = false
	obs.Party[0].Level = 60
	if got := gsProgressionCombatRecovery(obs, fight); len(got) > 0 {
		t.Fatalf("unbounded training offered past max level: %+v", got)
	}
}

func TestGSRecoveryTrainAndHealAreNativeOnly(t *testing.T) {
	a := newGSObjectiveAdapter(nil, nil, gsprofile.SilverGameID)
	for _, o := range []Objective{
		{Kind: KindTrain, Level: 39, Slot: 0},
		{Kind: KindHeal},
	} {
		if err := a.Validate(o, Observation{}); err != nil {
			t.Fatalf("validate %s: %v", o, err)
		}
		if budget := gsObjectiveFrameBudget(o); budget != gsFirstBadgeObjectiveFrameBudget {
			t.Fatalf("%s watchdog budget %d", o, budget)
		}
	}
	for _, o := range []Objective{
		{Kind: KindTrain, Level: 39, Slot: 1},
		{Kind: KindTrain, Level: 61},
		{Kind: KindTrain, Level: 39, Species: "meganium"},
		{Kind: KindHeal, Place: "violet city"},
	} {
		if err := a.Validate(o, Observation{}); !errors.Is(err, errGSControllerUnavailable) {
			t.Fatalf("invalid native objective %s: %v", o, err)
		}
	}
}

func TestGSRecoveryHealPostconditionSkipsEggButNotInjuredMon(t *testing.T) {
	a := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	o := Objective{Kind: KindHeal}
	obs := gsMortyRecoveryObservation(gsprofile.GoldGameID)
	obs.Party = append(obs.Party, PartyMon{Species: "egg", IsEgg: true})
	if err := a.VerifyPostcondition(o, Observation{}, obs, ObjectiveResult{}); err != nil {
		t.Fatalf("healed Gen2 party with egg rejected: %v", err)
	}
	obs.Party[0].HP--
	if err := a.VerifyPostcondition(o, Observation{}, obs, ObjectiveResult{}); !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("injured real mon passed heal: %v", err)
	}
	obs.Party[0].HP++
	obs.Controllable = false
	if err := a.VerifyPostcondition(o, Observation{}, obs, ObjectiveResult{}); !errors.Is(err, ErrObjectivePostconditionUnavailable) {
		t.Fatalf("unsettled heal passed: %v", err)
	}
}

// TestGSResumeGrindAfterInterruption pins the Gen2 grind invariant exposed by
// run-36loaokprjdtw2j8om666lswgf: a lone lead can black out to wild Confuse Ray
// self-hits. That wipe must heal-and-resume inside the grind session instead of
// surfacing as terminal required-win combat_defeat on the recovery train.
func TestGSResumeGrindAfterInterruption(t *testing.T) {
	wildLoss := skill.RequireBattleWin("combat recovery-grind", game.BattleLost)
	resume, afterBlackout := gsResumeGrindAfterInterruption(wildLoss)
	if !resume || !afterBlackout {
		t.Fatalf("wild blackout resume=%v afterBlackout=%v, want true/true", resume, afterBlackout)
	}

	trainerLoss := skill.RequireTrainerBattleWin("gym:bugsy", game.BattleLost)
	resume, afterBlackout = gsResumeGrindAfterInterruption(trainerLoss)
	if resume || afterBlackout {
		t.Fatalf("trainer loss resume=%v afterBlackout=%v, want false/false", resume, afterBlackout)
	}

	draw := skill.RequireBattleWin("combat recovery-grind", game.BattleDraw)
	resume, afterBlackout = gsResumeGrindAfterInterruption(draw)
	if !resume || afterBlackout {
		t.Fatalf("wild draw resume=%v afterBlackout=%v, want true/false", resume, afterBlackout)
	}

	resume, afterBlackout = gsResumeGrindAfterInterruption(fmt.Errorf("script stall"))
	if resume || afterBlackout {
		t.Fatalf("non-battle defect resume=%v afterBlackout=%v, want false/false", resume, afterBlackout)
	}
}
