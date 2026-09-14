package agent

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func TestRunWatchdogStagnationReplansOnceThenStops(t *testing.T) {
	known := NewKnowledge(nil)
	initial := Observation{Map: 1, X: 1, Y: 1, PartyCount: 1}
	known.SawMap(initial.Map)
	policy := newRunWatchdogPolicy(Budget{StagnationAfter: 2}, initial, known)

	// Keep moving so the dead-position watchdog does not mask stagnation.
	obs := initial
	obs.X = 2
	if got := policy.roundBoundary(2, obs, known, 0, true); got.Stop != StopUnset || got.ReplanReason != "" {
		t.Fatalf("round 2 decision = %+v; want continue", got)
	}
	obs.X = 3
	got := policy.roundBoundary(3, obs, known, 0, true)
	if got.Stop != StopUnset || got.ReplanReason != "stagnation" {
		t.Fatalf("first stagnation decision = %+v; want one strategic replan", got)
	}

	obs.X = 4
	if got := policy.roundBoundary(4, obs, known, 0, true); got.Stop != StopUnset {
		t.Fatalf("round 4 decision = %+v; want grace after replan", got)
	}
	obs.X = 5
	got = policy.roundBoundary(5, obs, known, 0, true)
	if got.Stop != StopStuck || got.Cause != runWatchdogStagnationRecurred {
		t.Fatalf("recurrent stagnation decision = %+v; want StopStuck", got)
	}
}

func TestRunWatchdogMajorProgressResetsStagnationEscalation(t *testing.T) {
	known := NewKnowledge(nil)
	initial := Observation{Map: 1, X: 1, Y: 1, PartyCount: 1}
	known.SawMap(initial.Map)
	policy := newRunWatchdogPolicy(Budget{StagnationAfter: 1}, initial, known)

	obs := initial
	obs.X = 2
	first := policy.roundBoundary(2, obs, known, 0, true)
	if first.ReplanReason != "stagnation" {
		t.Fatalf("first decision = %+v; want stagnation replan", first)
	}

	known.SawMap(2)
	obs.Map, obs.X = 2, 1
	progress := policy.roundBoundary(3, obs, known, 0, true)
	if !progress.MajorProgress || progress.Stop != StopUnset {
		t.Fatalf("progress decision = %+v; want reset", progress)
	}

	obs.X = 2
	again := policy.roundBoundary(4, obs, known, 0, true)
	if again.ReplanReason != "stagnation" || again.Stop != StopUnset {
		t.Fatalf("post-progress decision = %+v; want a fresh replan budget", again)
	}
}

func TestRunWatchdogDexOwnershipResetsStagnationWithFullParty(t *testing.T) {
	known := NewKnowledge(nil)
	initial := Observation{
		Map:          1,
		X:            1,
		Y:            1,
		PartyCount:   6,
		PokedexOwned: []SpeciesID{SpeciesID("bulbasaur")},
	}
	known.SawMap(initial.Map)
	policy := newRunWatchdogPolicy(Budget{StagnationAfter: 1}, initial, known)

	obs := initial
	obs.X = 2
	first := policy.roundBoundary(2, obs, known, 0, true)
	if first.ReplanReason != "stagnation" {
		t.Fatalf("first decision = %+v; want stagnation replan", first)
	}

	// Catching a new species with a full party sends it to storage, so the
	// party dimensions do not change. The owned Dex count is nevertheless
	// real Completionist/Dex progress and must reset the watchdog.
	obs.X = 3
	obs.PokedexOwned = append(obs.PokedexOwned, SpeciesID("pidgey"))
	progress := policy.roundBoundary(3, obs, known, 0, true)
	if !progress.MajorProgress || progress.Stop != StopUnset {
		t.Fatalf("dex progress decision = %+v; want reset", progress)
	}

	obs.X = 4
	again := policy.roundBoundary(4, obs, known, 0, true)
	if again.ReplanReason != "stagnation" || again.Stop != StopUnset {
		t.Fatalf("post-dex-progress decision = %+v; want a fresh replan budget", again)
	}
}

func TestRunWatchdogShortStuckReplansOnceThenStops(t *testing.T) {
	obs := Observation{Map: 1, X: 1, Y: 1, PartyCount: 1}
	policy := newRunWatchdogPolicy(Budget{StuckAfter: 2}, obs, NewKnowledge(nil))

	if got := policy.successfulObjective(obs, obs, true); got.Stop != StopUnset || got.ReplanReason != "" {
		t.Fatalf("first unchanged success = %+v; want continue", got)
	}
	got := policy.successfulObjective(obs, obs, true)
	if got.Stop != StopUnset || got.ReplanReason != "stuck" {
		t.Fatalf("threshold decision = %+v; want strategic replan", got)
	}
	if got := policy.successfulObjective(obs, obs, true); got.Stop != StopUnset {
		t.Fatalf("first post-replan unchanged success = %+v; want continue", got)
	}
	got = policy.successfulObjective(obs, obs, true)
	if got.Stop != StopStuck || got.Cause != runWatchdogShortStuck {
		t.Fatalf("recurrent stuck decision = %+v; want StopStuck", got)
	}
}

func TestRunFailurePolicyStrategicFailureReplansOncePerStructuredState(t *testing.T) {
	policy := newRunFailurePolicy(3)
	obj := Objective{Kind: KindGoTo, Place: "route 1"}
	result := ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "route_prerequisite_missing",
			Recoverable: true,
		},
		Final: Observation{Map: 1, X: 2, Y: 3},
	}

	first := policy.recoverable(obj, result, true, 0)
	if first.Stop != StopUnset || first.ReplanReason != "objective_failed" || !first.Recovered {
		t.Fatalf("first failure = %+v; want recovered strategic replan", first)
	}
	second := policy.recoverable(obj, result, true, 0)
	if second.Stop != StopFailed {
		t.Fatalf("same structured failure = %+v; want StopFailed", second)
	}
}

func TestRunFailurePolicyConsecutiveFailuresAndSuccessReset(t *testing.T) {
	policy := newRunFailurePolicy(2)
	firstObj := Objective{Kind: KindGoTo, Place: "route 1"}
	secondObj := Objective{Kind: KindGoTo, Place: "route 2"}
	first := ObjectiveResult{
		Objective: firstObj,
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "blocked_a",
			Recoverable: true,
		},
		Final: Observation{Map: 1},
	}
	second := ObjectiveResult{
		Objective: secondObj,
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "blocked_b",
			Recoverable: true,
		},
		Final: Observation{Map: 2},
	}

	if got := policy.recoverable(firstObj, first, false, 0); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("first failure = %+v; want recovered", got)
	}
	policy.success()
	if got := policy.recoverable(secondObj, second, false, 0); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("failure after success = %+v; success should reset streak", got)
	}
	if got := policy.recoverable(firstObj, first, false, 0); got.Stop != StopFailed {
		t.Fatalf("second consecutive failure = %+v; want StopFailed at budget", got)
	}
}

func TestRunFailurePolicyTrainingRetreatUsesLevelStreak(t *testing.T) {
	policy := newRunFailurePolicy(2)
	obj := Objective{Kind: KindTrain, Species: SpeciesID("pikachu"), Level: 10}
	result := ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "train_retreat",
			Recoverable: true,
		},
	}

	if got := policy.recoverable(obj, result, false, 7); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("first retreat = %+v; want recovered", got)
	}
	if got := policy.recoverable(obj, result, false, 8); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("retreat after level gain = %+v; want streak reset", got)
	}
	if got := policy.recoverable(obj, result, false, 8); got.Stop != StopFailed {
		t.Fatalf("same-level retreat = %+v; want StopFailed at streak budget", got)
	}
}
