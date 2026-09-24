package agent

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func TestRequiredBattleLossNormalizesAsGenericCombatDefeat(t *testing.T) {
	err := fmt.Errorf("story wrapper: %w", skill.RequireTrainerBattleWin("gym:blaine", state.ResultLost))
	failure := normalizeRedFailure(gameruntime.FailurePhaseExecution, err, Observation{Controllable: true})

	if failure.Class != gameruntime.FailureClassBlocked || !failure.Recoverable {
		t.Fatalf("failure = %+v, want recoverable blocked", failure)
	}
	if failure.Cause != failureCauseCombatDefeat {
		t.Fatalf("cause = %q, want %q", failure.Cause, failureCauseCombatDefeat)
	}
	if !reflect.DeepEqual(failure.Context, []string{"gym:blaine"}) {
		t.Fatalf("context = %v, want encounter identity", failure.Context)
	}
}

func TestRequiredNonTrainerBattleLossNormalizesAsCombatDefeat(t *testing.T) {
	err := skill.RequireBattleWin("static:route16_snorlax", state.ResultLost)
	failure := normalizeRedFailure(gameruntime.FailurePhaseExecution, err, Observation{Controllable: true})

	if failure.Class != gameruntime.FailureClassBlocked || !failure.Recoverable {
		t.Fatalf("failure = %+v, want recoverable blocked", failure)
	}
	if failure.Cause != failureCauseCombatDefeat {
		t.Fatalf("cause = %q, want %q", failure.Cause, failureCauseCombatDefeat)
	}
	if !reflect.DeepEqual(failure.Context, []string{"static:route16_snorlax"}) {
		t.Fatalf("context = %v, want encounter identity", failure.Context)
	}
	if errors.Is(err, skill.ErrTrainerBlackedOut) {
		t.Fatal("non-trainer required battle was mislabeled as a trainer blackout")
	}
}

func TestRequiredBattleDrawStaysDistinctFromBlackout(t *testing.T) {
	err := skill.RequireTrainerBattleWin("gym:blaine", state.ResultDraw)
	failure := normalizeRedFailure(gameruntime.FailurePhaseExecution, err, Observation{Controllable: true})

	if failure.Cause != failureCauseCombatNotWon || !failure.Recoverable {
		t.Fatalf("failure = %+v, want recoverable non-win", failure)
	}
	result := ObjectiveResult{Outcome: OutcomeBlocked, Failure: &failure}
	if failureIsBlackout(result) {
		t.Fatalf("draw was classified as blackout: %+v", failure)
	}
}

func TestRedOwnedRequiredBattleProjectsPortableEvidence(t *testing.T) {
	obj := Objective{Kind: KindProgress, Progress: "volcano_badge"}
	native := fmt.Errorf("progression wrapper: %w", skill.RequireTrainerBattleWin("gym:blaine", state.ResultLost))

	result, err := normalizeRedOwnedExecutionResult(obj, ObjectiveResult{Objective: obj}, native)
	if !errors.Is(err, skill.ErrTrainerBlackedOut) {
		t.Fatalf("native error = %v, want legacy blackout compatibility", err)
	}
	if result.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %q, want blocked", result.Outcome)
	}
	if result.Battle == nil || result.Battle.Encounter != "gym:blaine" || result.Battle.Result != "lost" || result.Battle.Won {
		t.Fatalf("battle evidence = %+v", result.Battle)
	}
}

func TestStructuredProgressionCombatLossUsesGenericRecoveryGate(t *testing.T) {
	obj := Objective{Kind: KindProgress, Progress: "volcano_badge"}
	result := ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Battle:    &BattleEvidence{Encounter: "gym:blaine", Result: "lost"},
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       failureCauseCombatDefeat,
			Recoverable: true,
		},
	}
	known := NewKnowledge(nil)
	known.FailedResult(result, errors.New("diagnostic wording is irrelevant"))

	if !combatLossRecorded(known, obj) {
		t.Fatal("structured progression loss did not create generic combat gate")
	}
	offered := filterCombatRecoveryBlocked([]Objective{
		{Kind: KindTrain, Level: 38},
		obj,
	}, known, ObjectiveCatalog{})
	if len(offered) != 1 || offered[0].Kind != KindTrain {
		t.Fatalf("offered after combat loss = %+v, want training without unchanged rechallenge", offered)
	}

	before := Observation{Party: []PartyMon{{Level: 37}}}
	after := Observation{Party: []PartyMon{{Level: 38}}}
	known.notePartyCombatResult(before, after, ObjectiveResult{})
	if combatLossRecorded(known, obj) {
		t.Fatal("material party progress did not clear generic combat gate")
	}
}

func TestStructuredGymCombatLossStillCreatesRetryDue(t *testing.T) {
	gym := Objective{Kind: KindGym, Place: "pewter gym"}
	known := NewKnowledge(nil)
	known.FailedResult(ObjectiveResult{
		Objective: gym,
		Outcome:   OutcomeBlocked,
		Battle:    &BattleEvidence{Encounter: "pewter gym", Result: "lost"},
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       failureCauseCombatDefeat,
			Recoverable: true,
		},
	}, errors.New("diagnostic"))

	if !combatLossRecorded(known, gym) {
		t.Fatal("generic structured gym loss did not enter gym recovery gate")
	}

	before := Observation{Party: []PartyMon{{Level: 10}}}
	after := Observation{Party: []PartyMon{{Level: 11}}}
	known.notePartyCombatResult(before, after, ObjectiveResult{})
	if !combatRetryKeys(known)[combatRecoveryObjective(gym).Key()] {
		t.Fatalf("gym retry not pending in generic combat state: %+v", known.Failures)
	}
}
