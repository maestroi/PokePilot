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

func TestRunFailurePolicyMarksProductiveBoundedSessions(t *testing.T) {
	cases := []struct {
		cause string
		want  bool
	}{
		{cause: "catch_hunt_exhausted", want: true},
		{cause: "fishing_hunt_exhausted", want: true},
		{cause: "train_progress_shortfall", want: true},
		{cause: "fishing_no_shoreline", want: false},
		{cause: "navigation_stalled", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.cause, func(t *testing.T) {
			policy := newRunFailurePolicy(3)
			obj := Objective{Kind: KindCatch, Species: SpeciesID("tentacool")}
			if tc.cause == "train_progress_shortfall" {
				obj = Objective{Kind: KindTrain, Level: 20}
			}
			result := recoverableSessionResult(obj, tc.cause, Observation{Map: 5, X: 12, Y: 23})
			got := policy.recoverable(obj, result, false, 20)
			if got.ProductiveSession != tc.want {
				t.Fatalf("%s ProductiveSession=%v, want %v (decision=%+v)", tc.cause, got.ProductiveSession, tc.want, got)
			}
		})
	}
}

func TestRunWatchdogProductiveSessionRefreshesLivenessWithoutMajorProgress(t *testing.T) {
	known := NewKnowledge(nil)
	initial := Observation{Map: 5, X: 12, Y: 23, PartyCount: 1}
	known.SawMap(initial.Map)
	watchdog := newRunWatchdogPolicy(Budget{StagnationAfter: 1}, initial, known)

	// One semantically unchanged round legitimately reaches the strategic
	// stagnation escalation point.
	first := watchdog.roundBoundary(2, initial, known, 0, true)
	if first.ReplanReason != "stagnation" || first.Stop != StopUnset {
		t.Fatalf("first stagnation decision = %+v, want strategic replan", first)
	}

	// A complete stochastic hunt session happened in round 2. It did not catch
	// the target species, so it is not major semantic progress, but it proves
	// the controller is doing useful bounded work and must refresh liveness.
	watchdog.productiveSession(2)

	next := watchdog.roundBoundary(3, initial, known, 0, true)
	if next.Stop != StopUnset || next.ReplanReason != "" {
		t.Fatalf("post-session watchdog decision = %+v, want continue", next)
	}
	if next.StagnantRounds != 0 {
		t.Fatalf("post-session stagnant rounds = %d, want 0", next.StagnantRounds)
	}
	if next.DeadStreak != 0 {
		t.Fatalf("post-session dead-position streak = %d, want 0", next.DeadStreak)
	}
	if next.MajorProgress {
		t.Fatal("productive stochastic session was incorrectly reported as semantic major progress")
	}
}
