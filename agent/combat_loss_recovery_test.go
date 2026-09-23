package agent

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func recoveredParty(level uint8) []PartyMon {
	return []PartyMon{{Species: "squirtle", Level: level, HP: 42, MaxHP: 42}}
}

func TestPromoteDefeatRespawnFailureMakesProgressionLossRecoverable(t *testing.T) {
	obj := Objective{Kind: KindProgress, Progress: "volcano_badge"}
	initial := Observation{
		Location: "cinnabar gym",
		X:        3, Y: 4,
		Money: 2400,
		Party: recoveredParty(37),
	}
	final := Observation{
		Location:     "cinnabar island",
		X:            3,
		Y:            4,
		Controllable: true,
		Money:        1200,
		RespawnPlace: "cinnabar island",
		Party:        recoveredParty(37),
	}
	result := ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeUnknownFailure,
		Failure: &gameruntime.Failure{
			Phase: gameruntime.FailurePhaseExecution,
			Class: gameruntime.FailureClassUnknown,
			Cause: "unknown_error",
		},
	}

	promoteDefeatRespawnFailure(&result, initial, final)

	if result.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %q, want blocked", result.Outcome)
	}
	if result.Failure == nil || result.Failure.Cause != failureCauseCombatDefeat || !result.Failure.Recoverable {
		t.Fatalf("failure = %+v, want recoverable combat defeat", result.Failure)
	}
	if !failureIsBlackout(result) {
		t.Fatalf("combat defeat did not enter generic blackout recovery: %+v", result)
	}
	if got := recoveryStateScopeFor(result); got != recoveryStateScopeCombatLoss {
		t.Fatalf("recovery scope = %v, want combat loss", got)
	}
}

func TestPromoteDefeatRespawnFailureDoesNotOverrideKnownFailure(t *testing.T) {
	initial := Observation{Location: "cinnabar gym", Money: 2400, Party: recoveredParty(37)}
	final := Observation{
		Location:     "cinnabar island",
		Controllable: true,
		Money:        1200,
		RespawnPlace: "cinnabar island",
		Party:        recoveredParty(37),
	}
	result := ObjectiveResult{
		Outcome: OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "route_prerequisite_missing",
			Recoverable: true,
		},
	}

	promoteDefeatRespawnFailure(&result, initial, final)
	if got := result.Failure.Cause; got != "route_prerequisite_missing" {
		t.Fatalf("known failure cause replaced with %q", got)
	}
}

func TestDefeatRespawnRequiresRealRespawnEvidence(t *testing.T) {
	initial := Observation{
		Location: "cinnabar island",
		X:        3, Y: 4,
		Money: 1200,
		Party: recoveredParty(37),
	}
	final := initial
	final.Controllable = true
	final.RespawnPlace = final.Location

	if defeatRespawned(initial, final) {
		t.Fatal("unchanged healthy Center state was mistaken for a combat defeat")
	}
}

func TestCombatDefeatDoesNotSpendMechanicalFailureBudget(t *testing.T) {
	policy := newRunFailurePolicy(2)
	obj := Objective{Kind: KindProgress, Progress: "volcano_badge"}
	result := strategicBlackoutResult(obj, failureCauseCombatDefeat)

	// A required-battle loss has its own combat-loss gate. Repeated observations
	// of that semantic outcome must keep replanning instead of exhausting the
	// generic controller/navigation failure budget.
	for i := 0; i < 5; i++ {
		got := policy.recoverable(obj, result, true, 0)
		if got.Stop != StopUnset || got.ReplanReason != "blackout" || !got.Recovered {
			t.Fatalf("combat defeat %d = %+v; want recovered blackout replan", i+1, got)
		}
	}

	// The defeat did not poison or consume the mechanical budget. A real
	// repeated controller failure still gets the normal one-shot/ceiling policy.
	mechanical := ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "navigation_stalled",
			Recoverable: true,
		},
	}
	if got := policy.recoverable(obj, mechanical, true, 0); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("first mechanical failure after combat defeats = %+v; want recovered", got)
	}
	if got := policy.recoverable(obj, mechanical, true, 0); got.Stop != StopFailed {
		t.Fatalf("repeated mechanical failure = %+v; want StopFailed", got)
	}
}
