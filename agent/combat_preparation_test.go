package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func combatPreparationTestObservation(levels ...uint8) Observation {
	obs := Observation{PartyCount: len(levels)}
	for i, level := range levels {
		obs.Party = append(obs.Party, PartyMon{
			Species: SpeciesID("testmon"),
			Level:   level,
			HP:      uint16(100 + i),
			MaxHP:   uint16(100 + i),
		})
	}
	return obs
}

func recordStructuredCombatLoss(t *testing.T, known *Knowledge, obj Objective, obs Observation) Failure {
	t.Helper()
	result := ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Final:     obs,
		Battle:    &BattleEvidence{Result: "lost"},
	}
	known.FailedResult(result, errors.New("required battle lost"))
	failure, ok := known.Failures[combatLossFailureKey(obj)]
	if !ok {
		t.Fatalf("combat loss was not recorded: %+v", known.Failures)
	}
	return failure
}

func TestStructuredCombatLossRequiresPreparationCampaignBeforeRetry(t *testing.T) {
	known := NewKnowledge(nil)
	obj := Objective{Kind: KindProgress, Progress: ProgressID("main_story_complete")}
	start := combatPreparationTestObservation(40)
	failure := recordStructuredCombatLoss(t, known, obj, start)

	if failure.Times != 1 {
		t.Fatalf("loss count = %d, want 1", failure.Times)
	}
	if failure.ReadinessBaseline != 160 || failure.ReadinessTarget != 180 {
		t.Fatalf("solo L40 readiness = %d -> %d, want 160 -> 180 (+5 lead-level equivalent)",
			failure.ReadinessBaseline, failure.ReadinessTarget)
	}

	// The old recovery gate retried after any +1/+2 improvement. A solo carry
	// now has to finish the whole first preparation campaign.
	mid := combatPreparationTestObservation(42)
	known.notePartyCombatResult(start, mid, ObjectiveResult{Objective: Objective{Kind: KindTrain, Level: 42}})
	if !combatLossRecorded(known, obj) {
		t.Fatal("combat loss gate opened after only two solo levels")
	}
	if combatRetryKeys(known)[combatRecoveryObjective(obj).Key()] {
		t.Fatal("retry became due before readiness target")
	}

	readyObs := combatPreparationTestObservation(45)
	known.notePartyCombatResult(mid, readyObs, ObjectiveResult{Objective: Objective{Kind: KindTrain, Level: 45}})
	if combatLossRecorded(known, obj) {
		t.Fatal("combat loss gate stayed locked after readiness target")
	}
	if !combatRetryKeys(known)[combatRecoveryObjective(obj).Key()] {
		t.Fatal("retry was not scheduled after completing preparation campaign")
	}
}

func TestRepeatedCombatLossEscalatesPreparationTarget(t *testing.T) {
	known := NewKnowledge(nil)
	obj := Objective{Kind: KindProgress, Progress: ProgressID("main_story_complete")}
	start := combatPreparationTestObservation(40)
	first := recordStructuredCombatLoss(t, known, obj, start)

	readyObs := combatPreparationTestObservation(45)
	known.notePartyCombatResult(start, readyObs, ObjectiveResult{Objective: Objective{Kind: KindTrain, Level: 45}})
	if !combatRetryKeys(known)[combatRecoveryObjective(obj).Key()] {
		t.Fatal("first preparation campaign did not schedule retry")
	}

	second := recordStructuredCombatLoss(t, known, obj, readyObs)
	if second.Times != 2 {
		t.Fatalf("second loss count = %d, want 2 carried through retry state", second.Times)
	}
	if second.ReadinessBaseline != 180 {
		t.Fatalf("second baseline = %d, want 180", second.ReadinessBaseline)
	}
	if second.ReadinessTarget-second.ReadinessBaseline <= first.ReadinessTarget-first.ReadinessBaseline {
		t.Fatalf("second campaign did not escalate: first +%d, second +%d",
			first.ReadinessTarget-first.ReadinessBaseline,
			second.ReadinessTarget-second.ReadinessBaseline)
	}
	if second.ReadinessTarget != 208 {
		t.Fatalf("second solo campaign target = %d, want 208 (L45 -> about L52)", second.ReadinessTarget)
	}
}

func TestCombatPreparationAllowsBroaderPartyProgress(t *testing.T) {
	known := NewKnowledge(nil)
	obj := Objective{Kind: KindProgress, Progress: ProgressID("main_story_complete")}
	start := combatPreparationTestObservation(40, 35, 30)
	failure := recordStructuredCombatLoss(t, known, obj, start)

	if failure.ReadinessBaseline != 260 || failure.ReadinessTarget != 272 {
		t.Fatalf("three-mon readiness = %d -> %d, want 260 -> 272",
			failure.ReadinessBaseline, failure.ReadinessTarget)
	}

	// Three levels on the lead is one way to satisfy the target, but not the
	// only way: useful secondary development contributes too.
	improved := combatPreparationTestObservation(41, 38, 32)
	known.notePartyCombatResult(start, improved, ObjectiveResult{Objective: Objective{Kind: KindTrain, Level: 38, Slot: 1}})
	if combatLossRecorded(known, obj) {
		t.Fatalf("broader party improvement readiness=%d did not release target=%d",
			partyCombatReadiness(improved), failure.ReadinessTarget)
	}
}

func TestCombatPreparationObjectiveKeepsLocalRecoveryDeterministic(t *testing.T) {
	known := NewKnowledge(nil)
	obj := Objective{Kind: KindProgress, Progress: ProgressID("main_story_complete")}
	obs := combatPreparationTestObservation(40)
	recordStructuredCombatLoss(t, known, obj, obs)

	offered := []Objective{
		{Kind: KindGoTo, Place: PlaceID("victory road")},
		{Kind: KindTrain, Level: 42},
	}
	got, ok := combatPreparationObjective(obs, offered, known)
	if !ok || got.Kind != KindTrain {
		t.Fatalf("combat preparation choice = %+v, %v; want local training", got, ok)
	}

	hurt := obs
	hurt.Party = append([]PartyMon(nil), obs.Party...)
	hurt.Party[0].HP = 10
	offered = []Objective{{Kind: KindHeal, Place: PlaceID("indigo plateau pokemon center")}, {Kind: KindTrain, Level: 42}}
	got, ok = combatPreparationObjective(hurt, offered, known)
	if !ok || got.Kind != KindHeal {
		t.Fatalf("hurt combat preparation choice = %+v, %v; want healing first", got, ok)
	}

	journeys := annotateCombatPreparation(obs, known, []Objective{{Kind: KindGoTo, Place: PlaceID("victory road")}})
	if len(journeys) != 1 || !strings.Contains(journeys[0].Note, "no viable local training") {
		t.Fatalf("journey annotation = %+v, want stronger-training-area guidance", journeys)
	}
	if _, ok := combatPreparationObjective(obs, journeys, known); ok {
		t.Fatal("combat preparation forced an arbitrary journey when no local training was viable")
	}
}

func TestCombatPreparationReadinessPersistsInCheckpointMemory(t *testing.T) {
	known := NewKnowledge(nil)
	obj := Objective{Kind: KindProgress, Progress: ProgressID("main_story_complete")}
	recordStructuredCombatLoss(t, known, obj, combatPreparationTestObservation(40))

	data, err := encodeMemoryFile(known, "prepare", 1)
	if err != nil {
		t.Fatal(err)
	}
	var mem memoryFile
	if err := json.Unmarshal(data, &mem); err != nil {
		t.Fatal(err)
	}
	restored := NewKnowledge(nil)
	restored.restore(mem)
	failure := restored.Failures[combatLossFailureKey(obj)]
	if failure.ReadinessBaseline != 160 || failure.ReadinessTarget != 180 || failure.Times != 1 {
		t.Fatalf("restored preparation failure = %+v, want readiness 160 -> 180 and one loss", failure)
	}
}

func TestCombatPreparationRestocksHealingBeforeTraining(t *testing.T) {
	known := NewKnowledge(nil)
	obj := Objective{Kind: KindProgress, Progress: ProgressID("main_story_complete")}
	obs := combatPreparationTestObservation(40)
	recordStructuredCombatLoss(t, known, obj, obs)
	if !known.hasCombatLossEvidence() {
		t.Fatal("typed combat loss not reported as loss evidence")
	}

	buy := Objective{Kind: KindBuy, Item: ItemID("super potion"), Qty: 3}
	offered := []Objective{{Kind: KindTrain, Level: 42}, buy}
	got, ok := combatPreparationObjective(obs, offered, known)
	if !ok || got.Kind != KindBuy {
		t.Fatalf("empty-bag preparation choice = %+v, %v; want healing restock first", got, ok)
	}

	stocked := obs
	stocked.Bag = []Item{{Name: "potion", Quantity: 1}}
	got, ok = combatPreparationObjective(stocked, offered, known)
	if !ok || got.Kind != KindTrain {
		t.Fatalf("stocked preparation choice = %+v, %v; want training once healing stock exists", got, ok)
	}

	known.Done(obj)
	if known.hasCombatLossEvidence() {
		t.Fatal("combat loss evidence survived the challenge win")
	}
}

// run-3r9pgu3arq0ls358ehdw2khxo8: an L89 lead met its readiness target, yet the
// League kept wiping the party because the bag had no healing and nothing
// outside a shop ever offered a purchase. Restocking is logistics, not
// training, so it must not wait on the readiness target.
func TestCombatPreparationRestocksHealingAfterReadinessTargetMet(t *testing.T) {
	known := NewKnowledge(nil)
	obj := Objective{Kind: KindProgress, Progress: ProgressID("main_story_complete")}
	obs := combatPreparationTestObservation(40)
	recordStructuredCombatLoss(t, known, obj, obs)

	strong := combatPreparationTestObservation(90)
	strong.CombatLossRecorded = true
	strong.Money = 20000
	strong.RestockStock = []string{"ultra ball", "full restore", "max potion", "revive"}
	if state := combatPreparationFor(known, strong); state.Current < state.Target {
		t.Fatalf("fixture must meet its readiness target: %+v", state)
	}

	offered := restockHealingObjectives(strong)
	if len(offered) != 1 || offered[0].Kind != KindBuy || offered[0].Intent != combatRecoverySupplyIntent {
		t.Fatalf("remote restock offer = %+v, want one travel-and-buy healing purchase", offered)
	}
	offered = append(offered, Objective{Kind: KindProgress, Progress: ProgressID("main_story_complete")})
	got, ok := combatPreparationObjective(strong, offered, known)
	if !ok || got.Kind != KindBuy {
		t.Fatalf("preparation choice = %+v, %v; want the healing restock before a retry", got, ok)
	}

	stocked := strong
	stocked.Bag = []Item{{Name: "max potion", Quantity: 2}}
	if offer := restockHealingObjectives(stocked); len(offer) != 0 {
		t.Fatalf("stocked bag still offered a restock: %+v", offer)
	}
	if got, ok := combatPreparationObjective(stocked, offered, known); ok {
		t.Fatalf("stocked party with met target was still forced into %+v", got)
	}
}

// A city cannot train and every learned habitat is outside the bounded session
// budget: preparation has no reachable path to its target, so the locked fight
// must become retryable instead of leaving the planner to wander between towns.
func TestCombatPreparationReleasesWhenNoTrainingPathExists(t *testing.T) {
	obj := Objective{Kind: KindProgress, Progress: ProgressID("main_story_complete")}
	unusable := TrainingAreaAssessment{
		Place: "route 3", Location: "route 3", Routable: true,
		Estimate: TrainingEstimate{Viability: TrainingOutsideBudget, XPPerEncounter: 51},
	}
	usable := unusable
	usable.Place, usable.Location, usable.Selected = "route 23", "route 23", true
	usable.Estimate.Viability = TrainingViable

	for _, tc := range []struct {
		name    string
		choices []TrainingAreaAssessment
		release bool
	}{
		{"no usable area", []TrainingAreaAssessment{unusable}, true},
		{"usable area known", []TrainingAreaAssessment{unusable, usable}, false},
		// run-2wka7km6oqsz62cajfp6yl6cuo: no assessed training areas at all
		// (the agent never learned a habitat) used to read as "not exhausted",
		// which deadlocked the preparation loop on heal/resupply/reposition.
		{"no areas assessed", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			known := NewKnowledge(nil)
			obs := combatPreparationTestObservation(36, 89, 27, 37)
			obs.Training = &TrainingEstimate{Viability: TrainingOutsideBudget}
			recordStructuredCombatLoss(t, known, obj, obs)
			obs.TrainingAreaChoices = tc.choices

			OfferWithEvidence(obs, known)
			if got := !combatLossRecorded(known, obj); got != tc.release {
				t.Fatalf("gate released = %v, want %v", got, tc.release)
			}
			if got := combatRetryKeys(known)[combatRecoveryObjective(obj).Key()]; got != tc.release {
				t.Fatalf("retry due = %v, want %v", got, tc.release)
			}
		})
	}
}

// campaignTestTrainingEstimate mirrors the measured live shape of
// run-1qtjk6v1dzvfam on Route 25: ~102 XP per encounter, 45 encounters for the
// next two lead levels and 127 for the whole escalated campaign gap.
func campaignTestTrainingEstimate(targetLevel uint8, xpRemaining uint32) TrainingEstimate {
	const xpPerEncounter = 102
	return TrainingEstimate{
		CurrentLevel: 27, TargetLevel: targetLevel, XPRemaining: xpRemaining,
		XPPerEncounter: xpPerEncounter, EstimatedEncounters: int((xpRemaining + xpPerEncounter - 1) / xpPerEncounter),
		SessionBudget: trainSessionBattleBudget, Viability: TrainingOutsideBudget, Method: TrainingDirect,
	}
}

func campaignTestObservation(t *testing.T, known *Knowledge, challenge Objective) Observation {
	t.Helper()
	obs := combatPreparationTestObservation(27, 26, 23) // readiness 183
	recordStructuredCombatLoss(t, known, challenge, obs)
	obs.HasGrass = true
	obs.WildGrass = []WildSpecies{{Name: "oddish", MinLevel: 12, MaxLevel: 14}}
	local := campaignTestTrainingEstimate(29, 4519)
	obs.Training = &local
	obs.TrainingAreaChoices = []TrainingAreaAssessment{{
		Place: "route 25", Location: "route 25", Routable: true,
		Estimate: campaignTestTrainingEstimate(32, 12898),
	}}
	return obs
}

// run-1qtjk6v1dzvfam: Cerulean Gym was lost twice with the party at readiness
// 179/199. Every assessed habitat needed more than one 20-battle session, so
// every habitat measured "outside budget", the loss was released as
// unreachable, and the run walked straight back into the fight it had just
// lost. A campaign that is allowed several sessions must keep the gate locked
// and offer a bounded training leg instead.
func TestCombatPreparationCampaignKeepsGateLockedAcrossSessions(t *testing.T) {
	known := NewKnowledge(nil)
	challenge := Objective{Kind: KindGym, Place: PlaceID("cerulean gym")}
	obs := campaignTestObservation(t, known, challenge)
	if failure := known.Failures[combatLossFailureKey(challenge)]; failure.ReadinessBaseline != 183 || failure.ReadinessTarget != 195 {
		t.Fatalf("campaign readiness = %d -> %d, want 183 -> 195", failure.ReadinessBaseline, failure.ReadinessTarget)
	}

	offer := OfferWithEvidence(obs, known)

	if !combatLossRecorded(known, challenge) {
		t.Fatal("gate opened below the campaign readiness target")
	}
	if combatRetryKeys(known)[combatRecoveryObjective(challenge).Key()] {
		t.Fatal("retry became due below the campaign readiness target")
	}
	trainingOffered := false
	for _, o := range offer.Candidates {
		if o.Kind == KindTrain {
			trainingOffered = true
		}
		if o.Kind == KindGym && o.Place == challenge.Place {
			t.Fatalf("lost challenge re-offered below its readiness target: %+v", o)
		}
	}
	if !trainingOffered {
		t.Fatalf("locked campaign offered no executable training leg: %+v", offer.Candidates)
	}

	// Reaching the target still releases the gate: retry is earned, not banned.
	ready := combatPreparationTestObservation(29, 28, 23) // readiness 195
	known.notePartyCombatResult(obs, ready, ObjectiveResult{Objective: Objective{Kind: KindTrain, Level: 29}})
	if !combatRetryKeys(known)[combatRecoveryObjective(challenge).Key()] {
		t.Fatal("retry was not scheduled once the campaign target was reached")
	}
}

// A habitat that would need dozens of bounded sessions is still a dead end, so
// the documented escape must keep firing: the locked fight becomes retryable
// rather than stranding the planner between towns
// (run-3r9pgu3arq0ls358ehdw2khxo8, an L89 lead over L2-L12 grass).
func TestCombatPreparationStillReleasesWhenNoHabitatClosesTheGap(t *testing.T) {
	known := NewKnowledge(nil)
	challenge := Objective{Kind: KindGym, Place: PlaceID("cerulean gym")}
	obs := campaignTestObservation(t, known, challenge)
	deadEnd := campaignTestTrainingEstimate(52, 45000) // ~442 encounters
	obs.Training = &deadEnd
	obs.TrainingAreaChoices = []TrainingAreaAssessment{{
		Place: "route 2", Location: "route 2", Routable: true, Estimate: deadEnd,
	}}

	OfferWithEvidence(obs, known)

	if combatLossRecorded(known, challenge) {
		t.Fatal("dead-end campaign kept the gate locked")
	}
	if !combatRetryKeys(known)[combatRecoveryObjective(challenge).Key()] {
		t.Fatal("dead-end campaign did not release the fight to retry")
	}

	// Walking into the gym's city (no local grass, so "not exhausted") must
	// not demote the escape back into the unmet campaign: that flip-flop
	// commuted run-j1lgrfvixz7j1bgxlgsfptofu between Route 22 and Viridian.
	city := obs
	city.Training = nil
	OfferWithEvidence(city, known)
	if !combatRetryKeys(known)[combatRecoveryObjective(challenge).Key()] {
		t.Fatal("city offer re-locked a dead-end retry")
	}
}

// run-1qtjk6v1dzvfam's knowledge already held a retry marker that an older
// build wrote while the party was far below its recorded target. Retry mode
// means "the target is met, test the stronger party"; an unmet marker has to
// return to the campaign or the same lost fight is re-offered forever.
func TestStaleRetryMarkerReturnsToPreparationCampaign(t *testing.T) {
	known := NewKnowledge(nil)
	challenge := Objective{Kind: KindGym, Place: PlaceID("cerulean gym")}
	obs := campaignTestObservation(t, known, challenge)
	delete(known.Failures, combatLossFailureKey(challenge))
	known.Failures[combatRetryReadyKey(challenge)] = Failure{
		Objective: challenge.String(), Times: 2, ReadinessBaseline: 179, ReadinessTarget: 199,
		Last: "combat preparation target reached after defeat; retry is due",
	}

	OfferWithEvidence(obs, known)

	if !combatLossRecorded(known, challenge) {
		t.Fatal("stale retry marker did not return to the preparation campaign")
	}
	if combatRetryKeys(known)[combatRecoveryObjective(challenge).Key()] {
		t.Fatal("stale retry marker stayed retry-ready below its target")
	}
	demoted := known.Failures[combatLossFailureKey(challenge)]
	if demoted.Times != 2 || demoted.ReadinessTarget != 199 {
		t.Fatalf("demoted campaign = %+v, want the original 2 losses and target 199", demoted)
	}
	if strings.Contains(demoted.Last, "retry is due") {
		t.Fatalf("demoted campaign kept the stale retry text %q", demoted.Last)
	}

	// A marker the party has actually reached stays retry-ready.
	reached := NewKnowledge(nil)
	reached.Failures[combatRetryReadyKey(challenge)] = Failure{
		Objective: challenge.String(), Times: 1, ReadinessBaseline: 179, ReadinessTarget: 183,
		Last: "combat preparation target reached after defeat; retry is due",
	}
	OfferWithEvidence(obs, reached)
	if !combatRetryKeys(reached)[combatRecoveryObjective(challenge).Key()] {
		t.Fatal("satisfied retry marker was demoted back into preparation")
	}
}

func TestCampaignTrainingBudgetOnlyWidensMeasuredEstimates(t *testing.T) {
	known := NewKnowledge(nil)
	challenge := Objective{Kind: KindGym, Place: PlaceID("cerulean gym")}
	obs := campaignTestObservation(t, known, challenge)

	if got := preparationTrainingBudget(); got != trainSessionBattleBudget*preparationSessionCeiling {
		t.Fatalf("campaign budget = %d, want %d", got, trainSessionBattleBudget*preparationSessionCeiling)
	}

	gap := campaignTestTrainingEstimate(32, 12898)
	budgeted := budgetedTrainingEstimate(&gap, obs, known)
	if budgeted == &gap {
		t.Fatal("measured estimate was not re-read against the campaign budget")
	}
	if budgeted.Viability != TrainingExpensive || budgeted.SessionBudget != preparationTrainingBudget() {
		t.Fatalf("budgeted estimate = %q/%d, want expensive/%d",
			budgeted.Viability, budgeted.SessionBudget, preparationTrainingBudget())
	}

	unsafe := TrainingEstimate{Viability: TrainingOutsideBudget}
	if got := budgetedTrainingEstimate(&unsafe, obs, known); got != &unsafe {
		t.Fatal("a band with no measured XP was re-priced")
	}
}

// TestBudgetedTrainingEstimateKeepsSingleSessionContract is the regression for
// run-otfpwf3802q31tv1pju7x3mme: a habitat whose full target fits the campaign's
// multi-session allowance but whose next level needs more than one bounded
// session. The executor refuses such a session (it would deliver zero levels),
// so the campaign re-pricing must keep the single-session OutsideBudget verdict
// instead of flipping it to viable and forcing a guaranteed-blocked train.
func TestBudgetedTrainingEstimateKeepsSingleSessionContract(t *testing.T) {
	known := NewKnowledge(nil)
	challenge := Objective{Kind: KindGym, Place: PlaceID("cerulean gym")}
	obs := campaignTestObservation(t, known, challenge)

	// The full target (10000 XP) fits the 160-battle campaign allowance
	// (~99 encounters), so a naive re-pricing would call it viable. But the
	// next level alone needs 2500 XP (~25 encounters), more than one 20-battle
	// session can deliver, so no session here makes progress.
	const xpPerEncounter = 102
	stalled := TrainingEstimate{
		CurrentLevel: 49, TargetLevel: 51, XPRemaining: 10000,
		XPPerEncounter:      xpPerEncounter,
		EstimatedEncounters: int((10000 + xpPerEncounter - 1) / xpPerEncounter),
		NextLevelXP:         2500,
		NextLevelEncounters: int((2500 + xpPerEncounter - 1) / xpPerEncounter),
		SessionBudget:       trainSessionBattleBudget,
		Viability:           TrainingOutsideBudget,
		Method:              TrainingDirect,
	}
	if stalled.NextLevelEncounters <= trainSessionBattleBudget {
		t.Fatalf("test setup: next level needs %d encounters, want > %d", stalled.NextLevelEncounters, trainSessionBattleBudget)
	}
	budgeted := budgetedTrainingEstimate(&stalled, obs, known)
	if budgeted.Viability != TrainingOutsideBudget {
		t.Fatalf("stalled habitat re-priced to %q, want outside_budget (next level out of one session)", budgeted.Viability)
	}

	// A habitat that CAN deliver one level per session keeps the campaign
	// widening: the full target fits the allowance and the next level fits one
	// session, so the re-pricing legitimately moves it into the viable class.
	flowing := TrainingEstimate{
		CurrentLevel: 49, TargetLevel: 51, XPRemaining: 10000,
		XPPerEncounter:      xpPerEncounter,
		EstimatedEncounters: int((10000 + xpPerEncounter - 1) / xpPerEncounter),
		NextLevelXP:         1500,
		NextLevelEncounters: int((1500 + xpPerEncounter - 1) / xpPerEncounter),
		SessionBudget:       trainSessionBattleBudget,
		Viability:           TrainingOutsideBudget,
		Method:              TrainingDirect,
	}
	if flowing.NextLevelEncounters > trainSessionBattleBudget {
		t.Fatalf("test setup: next level needs %d encounters, want <= %d", flowing.NextLevelEncounters, trainSessionBattleBudget)
	}
	if got := budgetedTrainingEstimate(&flowing, obs, known); got.Viability == TrainingOutsideBudget {
		t.Fatalf("productive habitat stayed outside_budget; the campaign widening was lost")
	}
}

func TestTrainingSessionExecutableOnlyBlocksBandsWithoutXP(t *testing.T) {
	for _, tc := range []struct {
		name     string
		estimate TrainingEstimate
		err      error
		want     bool
	}{
		{"viable session", TrainingEstimate{Viability: TrainingViable, XPPerEncounter: 40}, nil, true},
		{"multi-session band", TrainingEstimate{Viability: TrainingOutsideBudget, XPPerEncounter: 102}, nil, true},
		{"unsafe band with no carry", TrainingEstimate{Viability: TrainingOutsideBudget}, nil, false},
		{"unmeasurable habitat", TrainingEstimate{}, errors.New("no party"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := trainingSessionExecutable(tc.estimate, tc.err); got != tc.want {
				t.Fatalf("trainingSessionExecutable = %v, want %v", got, tc.want)
			}
		})
	}
}
