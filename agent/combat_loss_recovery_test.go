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
		X: 3, Y: 4,
		Money: 2400,
		Party: recoveredParty(37),
	}
	final := Observation{
		Location: "cinnabar island",
		X: 3, Y: 4,
		Controllable: true,
		Money: 1200,
		RespawnPlace: "cinnabar island",
		Party: recoveredParty(37),
	}
	result := ObjectiveResult{
		Objective: obj,
		Outcome: OutcomeUnknownFailure,
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
		Location: "cinnabar island",
		Controllable: true,
		Money: 1200,
		RespawnPlace: "cinnabar island",
		Party: recoveredParty(37),
	}
	result := ObjectiveResult{
		Outcome: OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class: gameruntime.FailureClassBlocked,
			Cause: "route_prerequisite_missing",
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
		X: 3, Y: 4,
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

func TestCombatDefeatUsesBoundedBlackoutPolicy(t *testing.T) {
	policy := newRunFailurePolicy(2)
	obj := Objective{Kind: KindProgress, Progress: "volcano_badge"}
	result := strategicBlackoutResult(obj, failureCauseCombatDefeat)

	for i := 0; i < 2; i++ {
		got := policy.recoverable(obj, result, true, 0)
		if got.Stop != StopUnset || got.ReplanReason != "blackout" || !got.Recovered {
			t.Fatalf("combat defeat %d = %+v; want bounded blackout recovery", i+1, got)
		}
	}
	if got := policy.recoverable(obj, result, true, 0); got.Stop != StopFailed {
		t.Fatalf("third combat defeat = %+v, want recovery ceiling", got)
	}
}

func TestCombatLossFilterFailsOpenForOnlyLegalRetry(t *testing.T) {
	known := NewKnowledge(nil)
	obj := Objective{Kind: KindTrainer, Location: "route 3", X: 10, Y: 6}
	known.Failures[trainerLossFailureKey(obj)] = Failure{
		Objective: obj.String(),
		Times: 1,
		Last: "lost battle",
	}

	got := filterTrainerLossBlocked([]Objective{obj}, known)
	if len(got) != 1 || got[0].Key() != obj.Key() {
		t.Fatalf("only legal combat retry was hidden: %v", got)
	}
}
