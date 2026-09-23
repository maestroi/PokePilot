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
			Level: level,
			HP: uint16(100 + i),
			MaxHP: uint16(100 + i),
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
