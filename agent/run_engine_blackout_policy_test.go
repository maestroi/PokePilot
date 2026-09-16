package agent

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func strategicBlackoutResult(obj Objective, cause string) ObjectiveResult {
	return ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       cause,
			Recoverable: true,
		},
		Final: Observation{
			Map:          0x01,
			MapName:      "VIRIDIAN_CITY",
			X:            23,
			Y:            26,
			Controllable: true,
		},
	}
}

func TestRunFailurePolicyOrdinaryTravelBlackoutDoesNotSpendPermanentEscalation(t *testing.T) {
	policy := newRunFailurePolicy(3)
	obj := Objective{Kind: KindGoTo, Place: "pewter city"}
	result := strategicBlackoutResult(obj, "blacked_out")

	first := policy.recoverable(obj, result, true, 0)
	if first.Stop != StopUnset || first.ReplanReason != "blackout" || !first.Recovered {
		t.Fatalf("first blackout = %+v; want recovered strategic replan", first)
	}

	// Issue #792: the same stochastic Travel blackout recurred after other
	// successful rounds and the durable strategic fingerprint immediately
	// terminated the run. Success resets the consecutive budget, so the same
	// ordinary blackout must remain recoverable rather than inheriting a
	// permanent one-shot escalation.
	policy.success()
	second := policy.recoverable(obj, result, true, 0)
	if second.Stop != StopUnset || second.ReplanReason != "blackout" || !second.Recovered {
		t.Fatalf("blackout after success = %+v; want another recovered replan", second)
	}
}

func TestRunFailurePolicyOrdinaryTravelBlackoutStillHasConsecutiveCeiling(t *testing.T) {
	policy := newRunFailurePolicy(2)
	obj := Objective{Kind: KindGoTo, Place: "pewter city"}
	result := strategicBlackoutResult(obj, "blacked_out")

	for i := 0; i < 2; i++ {
		got := policy.recoverable(obj, result, true, 0)
		if got.Stop != StopUnset || got.ReplanReason != "blackout" || !got.Recovered {
			t.Fatalf("blackout %d = %+v; want bounded recovered replan", i+1, got)
		}
	}
	if got := policy.recoverable(obj, result, true, 0); got.Stop != StopFailed {
		t.Fatalf("third consecutive blackout = %+v; want StopFailed at recovery ceiling", got)
	}
}

func TestRunFailurePolicyTrainerBlackoutRetainsDurableSameStateGate(t *testing.T) {
	policy := newRunFailurePolicy(3)
	obj := Objective{Kind: KindGoTo, Place: "pewter city", Flee: true}
	result := strategicBlackoutResult(obj, "trainer_blacked_out")

	first := policy.recoverable(obj, result, true, 0)
	if first.Stop != StopUnset || first.ReplanReason != "blackout" || !first.Recovered {
		t.Fatalf("first trainer blackout = %+v; want recovered strategic replan", first)
	}
	if got := policy.recoverable(obj, result, true, 0); got.Stop != StopFailed {
		t.Fatalf("same-state trainer blackout = %+v; want durable StopFailed gate", got)
	}
}
