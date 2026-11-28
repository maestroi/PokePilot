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
	for _, cause := range []string{"catch_hunt_exhausted", "fishing_hunt_exhausted", "catch_attempt_missed", "static_capture_exhausted"} {
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
	for _, cause := range []string{"catch_hunt_exhausted", "fishing_hunt_exhausted", "catch_attempt_missed", "static_capture_exhausted"} {
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
		{cause: "catch_attempt_missed", want: true},
		{cause: "static_capture_exhausted", want: true},
		{cause: "train_progress_shortfall", want: true},
		{cause: "train_retreat", want: true},
		{cause: failureCauseCombatDefeat, want: true},
		{cause: "fishing_no_shoreline", want: false},
		{cause: "navigation_stalled", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.cause, func(t *testing.T) {
			policy := newRunFailurePolicy(3)
			obj := Objective{Kind: KindCatch, Species: SpeciesID("tentacool")}
			if tc.cause == "train_progress_shortfall" || tc.cause == "train_retreat" {
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

func TestRunFailurePolicyLevelGainingTrainRetreatResetsFailureBudget(t *testing.T) {
	policy := newRunFailurePolicy(2)
	blocked := Objective{Kind: KindGoTo, Place: "cinnabar island"}
	train := Objective{Kind: KindTrain, Level: 40}

	first := recoverableSessionResult(blocked, "blocked_a", Observation{Location: "pokemon mansion b1f", Map: 0xd8, X: 23, Y: 21})
	if got := policy.recoverable(blocked, first, true, 0); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("first ordinary failure = %+v; want recovered", got)
	}

	retreat := recoverableSessionResult(train, "train_retreat", Observation{Location: "pokemon mansion b1f", Map: 0xd8, X: 23, Y: 21})
	retreat.Train = &TrainingEvidence{StartLevel: 38, EndLevel: 39, Battles: 6, Retreated: true}
	got := policy.recoverable(train, retreat, true, 39)
	if got.Stop != StopUnset || !got.Recovered || !got.ProductiveSession || got.ReplanReason != "train_retreat" {
		t.Fatalf("level-gaining train retreat = %+v; want productive train_retreat replan", got)
	}

	second := recoverableSessionResult(blocked, "blocked_b", Observation{Location: "pokemon mansion b1f", Map: 0xd8, X: 23, Y: 21})
	if got := policy.recoverable(blocked, second, true, 0); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("failure after level-gaining retreat = %+v; productive retreat must reset consecutive budget", got)
	}
}

func TestTrainRetreatProductiveSessionClearsDeadPositionWatchdog(t *testing.T) {
	known := NewKnowledge(nil)
	tile := Observation{Map: 0xd8, X: 23, Y: 21, PartyCount: 1, Party: []PartyMon{{Species: SpeciesID("blastoise"), Level: 39}}}
	known.SawMap(tile.Map)
	watchdog := newRunWatchdogPolicy(Budget{StagnationAfter: 200}, tile, known)
	policy := newRunFailurePolicy(3)
	train := Objective{Kind: KindTrain, Level: 40}

	for round := 1; round <= deadPositionAfter+1; round++ {
		retreat := recoverableSessionResult(train, "train_retreat", tile)
		retreat.Train = &TrainingEvidence{StartLevel: 38 + round - 1, EndLevel: 39 + round - 1, Battles: 4, Retreated: true}
		decision := policy.recoverable(train, retreat, true, uint8(retreat.Train.EndLevel))
		if !decision.ProductiveSession {
			t.Fatalf("round %d train retreat ProductiveSession=false; dead-position would stop a healthy grind", round)
		}
		watchdog.productiveSession(round)
		got := watchdog.roundBoundary(round+1, tile, known, 0, true)
		if got.Stop != StopUnset {
			t.Fatalf("round %d watchdog = %+v; productive train retreat must not trip dead-position", round, got)
		}
	}
}

func TestCombatDefeatRefreshesLivenessBeforeRecoveryRound(t *testing.T) {
	known := NewKnowledge(nil)
	initial := Observation{
		Map:        0x40,
		Location:   "cerulean pokemon center",
		PartyCount: 1,
		Party:      []PartyMon{{Species: SpeciesID("wartortle"), Level: 21}},
	}
	known.SawMap(initial.Map)
	watchdog := newRunWatchdogPolicy(Budget{StagnationAfter: 1}, initial, known)

	first := watchdog.roundBoundary(2, initial, known, 0, true)
	if first.ReplanReason != "stagnation" || first.Stop != StopUnset {
		t.Fatalf("pre-defeat stagnation decision = %+v, want strategic replan", first)
	}

	obj := Objective{Kind: KindGym, Place: PlaceID("cerulean gym")}
	result := recoverableSessionResult(obj, failureCauseCombatDefeat, initial)
	decision := newRunFailurePolicy(3).recoverable(obj, result, true, 21)
	if !decision.Recovered || !decision.ProductiveSession || decision.ReplanReason != "blackout" {
		t.Fatalf("combat defeat decision = %+v, want recovered productive blackout replan", decision)
	}
	watchdog.productiveSession(2)

	next := watchdog.roundBoundary(3, initial, known, 0, true)
	if next.Stop != StopUnset || next.ReplanReason != "" || next.StagnantRounds != 0 {
		t.Fatalf("first combat-recovery round was preempted by stagnation: %+v", next)
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

func TestRunFailurePolicyStaticUnavailableDoesNotSpendFailureBudget(t *testing.T) {
	policy := newRunFailurePolicy(2)
	obj := Objective{Kind: KindCatch, Species: SpeciesID("zapdos"), Place: "power plant zapdos", Intent: dexStaticIntent}
	result := recoverableSessionResult(obj, "static_capture_unavailable", Observation{Location: "power plant"})
	for i := 0; i < 5; i++ {
		got := policy.recoverable(obj, result, true, 0)
		if got.Stop != StopUnset || !got.Recovered || got.ReplanReason != "objective_failed" {
			t.Fatalf("static unavailable %d = %+v; want planning-boundary replan", i+1, got)
		}
	}
}
