package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
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
