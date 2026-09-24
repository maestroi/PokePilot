package agent

import "testing"

// A three-stage gauntlet declared by adapter data: entering commits to every
// stage, and no free recovery exists between the fights.
func challengeChainTestObservation(levels ...uint8) (Observation, Objective, Objective) {
	entry := Objective{Kind: KindProgress, Progress: ProgressID("gauntlet_started")}
	stage := Objective{Kind: KindProgress, Progress: ProgressID("gauntlet_boss_defeated")}
	profile := ChallengeReadinessProfile{MinimumReadiness: 100, MinimumSupportLevel: 45, RecoveryFights: 3}
	obs := combatPreparationTestObservation(levels...)
	obs.Money = 20000
	obs.MartStock = []string{"potion", "super potion", "revive"}
	obs.Catalog = ObjectiveCatalog{ChallengeProfiles: []CatalogChallengeProfile{
		{Objective: entry.Key(), Readiness: profile, Chain: "gauntlet"},
		{Objective: stage.Key(), Readiness: profile, Chain: "gauntlet"},
	}}
	return obs, entry, stage
}

func TestChainLossLocksChainEntry(t *testing.T) {
	known := NewKnowledge(nil)
	obs, entry, stage := challengeChainTestObservation(60, 50, 50)
	recordStructuredCombatLoss(t, known, stage, obs)

	offer := OfferWithProgressionEvidence(obs, known, fixedProgressionPlanner{entry})
	for _, o := range offer.Candidates {
		if o.Key() == entry.Key() {
			t.Fatalf("chain entry offered after a loss inside the chain: %+v", offer.Candidates)
		}
	}
	blocked := false
	for _, b := range offer.Blocked {
		blocked = blocked || (b.Objective != nil && *b.Objective == entry.Key() && b.Reason == "combat_readiness")
	}
	if !blocked {
		t.Fatalf("locked entry has no combat_readiness evidence: %+v", offer.Blocked)
	}

	// Once readiness promotes the loss to retry-ready, the entry is the retry
	// and the loss still counts for readiness logistics.
	known.promoteCombatLossesToRetry()
	if prep := challengePreparationFor(known, obs, entry); prep.Losses == 0 {
		t.Fatalf("retry-ready chain loss not visible to readiness: %+v", prep)
	}
	offer = OfferWithProgressionEvidence(obs, known, fixedProgressionPlanner{entry})
	if !hasObjectiveKey(offer.Candidates, entry.Key()) {
		t.Fatalf("retry-ready chain entry not offered: %+v", offer.Candidates)
	}
}

func TestChainStocksHealingBeforeCommitting(t *testing.T) {
	known := NewKnowledge(nil)
	obs, entry, _ := challengeChainTestObservation(60, 50, 50)

	offer := OfferWithProgressionEvidence(obs, known, fixedProgressionPlanner{entry})
	if offer.RecoveryFightsAhead != 3 {
		t.Fatalf("recovery fights ahead = %d, want 3", offer.RecoveryFightsAhead)
	}
	var heal, revive *Objective
	for i, o := range offer.Candidates {
		if o.Kind != KindBuy {
			continue
		}
		if _, ok := hpHealingItems[string(o.Item)]; ok {
			heal = &offer.Candidates[i]
		}
		if o.Item == "revive" {
			revive = &offer.Candidates[i]
		}
	}
	if heal == nil || heal.Qty != chainHealTarget(3) {
		t.Fatalf("heal buy = %+v, want %d for a 3-fight chain; offer=%+v", heal, chainHealTarget(3), offer.Candidates)
	}
	if revive == nil || revive.Qty != chainReviveTarget(3) {
		t.Fatalf("revive buy = %+v, want %d", revive, chainReviveTarget(3))
	}

	got, readiness, ok := proactiveChallengePreparationObjective(obs, offer.Candidates, offer.Readiness, known)
	if !ok || got.Kind != KindBuy || readiness.Action != ChallengeRestock {
		t.Fatalf("proactive choice = %+v (%+v), %v; want restock before the chain", got, readiness, ok)
	}

	stocked := obs
	stocked.Bag = []Item{{Name: "super potion", Quantity: chainHealTarget(3)}}
	if r := EvaluateChallengeReadiness(stocked, known, entry, challengeProfileFor(stocked, entry)); r.Action == ChallengeRestock {
		t.Fatalf("stocked party still asked to restock: %+v", r)
	}
}

func TestChainTrainsWeakSupportNotCarry(t *testing.T) {
	known := NewKnowledge(nil)
	// One L88 carry clears the lead-weighted readiness floor alone; the
	// bench (slots 1-3) does not clear the support floor.
	obs, entry, _ := challengeChainTestObservation(88, 33, 18, 32)
	obs.Bag = []Item{{Name: "super potion", Quantity: 9}, {Name: "revive", Quantity: 2}}

	r := EvaluateChallengeReadiness(obs, known, entry, challengeProfileFor(obs, entry))
	if r.Action != ChallengeTrain || r.TrainSlot != 3 {
		t.Fatalf("readiness = %+v, want train of weakest top support (slot 3, L32)", r)
	}
	offered := []Objective{
		{Kind: KindTrain, Level: 90},
		{Kind: KindTrain, Level: 34, Species: "testmon", Slot: 1},
		{Kind: KindTrain, Level: 33, Species: "testmon", Slot: 3},
		entry,
	}
	got, _, ok := proactiveChallengePreparationObjective(obs, offered, []ChallengeReadiness{r}, known)
	if !ok || got.Slot != 3 {
		t.Fatalf("training choice = %+v, %v; want slot 3", got, ok)
	}
}

func hasObjectiveKey(out []Objective, key ObjectiveKey) bool {
	for _, o := range out {
		if o.Key() == key {
			return true
		}
	}
	return false
}

func TestChainSupportTrainingFailsOpenWithoutSlotTraining(t *testing.T) {
	known := NewKnowledge(nil)
	obs, entry, _ := challengeChainTestObservation(88, 33, 18, 32)
	obs.Bag = []Item{{Name: "super potion", Quantity: 9}}
	r := EvaluateChallengeReadiness(obs, known, entry, challengeProfileFor(obs, entry))
	// Only the carry can train here: grinding it never fixes the bench.
	offered := []Objective{{Kind: KindTrain, Level: 90}, entry}
	if got, _, ok := proactiveChallengePreparationObjective(obs, offered, []ChallengeReadiness{r}, known); ok {
		t.Fatalf("support gap forced %+v; want the planner to choose", got)
	}
}
