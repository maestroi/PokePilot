package agent

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func recoverableSessionResult(obj Objective, cause string, final Observation) ObjectiveResult {
	return ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       cause,
			Recoverable: true,
		},
		Final: final,
	}
}

func TestRunFailurePolicyHuntExhaustionDoesNotSpendFailureBudget(t *testing.T) {
	for _, cause := range []string{"catch_hunt_exhausted", "fishing_hunt_exhausted"} {
		policy := newRunFailurePolicy(2)
		obj := Objective{Kind: KindCatch, Species: SpeciesID("nidoran♀")}
		result := recoverableSessionResult(obj, cause, Observation{
			Location:   "route 22",
			PartyCount: 1,
			Party:      []PartyMon{{Species: SpeciesID("bulbasaur"), Level: 12}},
		})

		for i := 0; i < 4; i++ {
			got := policy.recoverable(obj, result, false, 12)
			if got.Stop != StopUnset || !got.Recovered {
				t.Fatalf("%s miss %d = %+v; bounded stochastic miss must remain recoverable", cause, i+1, got)
			}
		}
	}
}

func TestRunFailurePolicyHuntExhaustionStillReplansStrategically(t *testing.T) {
	for _, cause := range []string{"catch_hunt_exhausted", "fishing_hunt_exhausted"} {
		policy := newRunFailurePolicy(2)
		obj := Objective{Kind: KindCatch, Species: SpeciesID("nidoran♀")}
		result := recoverableSessionResult(obj, cause, Observation{Location: "route 22"})

		for i := 0; i < 3; i++ {
			got := policy.recoverable(obj, result, true, 0)
			if got.Stop != StopUnset || !got.Recovered || got.ReplanReason != "objective_failed" {
				t.Fatalf("strategic %s miss %d = %+v; want recovered replan without escalation stop", cause, i+1, got)
			}
		}
	}
}

func TestRunFailurePolicyTrainProgressResetsConsecutiveFailureBudget(t *testing.T) {
	policy := newRunFailurePolicy(2)
	blockedA := Objective{Kind: KindGoTo, Place: "route 1"}
	blockedB := Objective{Kind: KindGoTo, Place: "route 2"}
	train := Objective{Kind: KindTrain, Level: 15}

	first := recoverableSessionResult(blockedA, "blocked_a", Observation{Location: "pallet town"})
	if got := policy.recoverable(blockedA, first, false, 0); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("first ordinary failure = %+v; want recovered", got)
	}

	progress := recoverableSessionResult(train, "train_progress_shortfall", Observation{
		Location:   "viridian forest",
		PartyCount: 1,
		Party:      []PartyMon{{Species: SpeciesID("bulbasaur"), Level: 14}},
	})
	if got := policy.recoverable(train, progress, false, 14); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("productive training shortfall = %+v; want recovered", got)
	}

	second := recoverableSessionResult(blockedB, "blocked_b", Observation{Location: "viridian city"})
	if got := policy.recoverable(blockedB, second, false, 0); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("failure after productive training = %+v; progress should reset consecutive budget", got)
	}
}
