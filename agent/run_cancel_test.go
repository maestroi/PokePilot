package agent

import (
	"testing"
)

func TestRunStopsOnCancelBetweenRounds(t *testing.T) {
	cancel := make(chan struct{})
	close(cancel) // already cancelled: must stop before round 1's objective runs
	planner := NewScriptedPlanner(Objective{Kind: KindGoTo, Place: "pallet town"})
	res := Run(nil, nil, planner, Budget{
		MaxRounds: 5, MaxFrames: 1000, Cancel: cancel,
	})
	if res.Stop != StopBudget {
		t.Fatalf("Stop = %v, want StopBudget (cancelled before any objective ran)", res.Stop)
	}
	if res.Rounds != 0 {
		t.Fatalf("Rounds = %d, want 0", res.Rounds)
	}
}
