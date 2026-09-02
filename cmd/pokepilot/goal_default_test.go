package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/red/state"
)

// The default goal must be a STRUCTURED goal, not prose. MEASURED
// 2026-09-02: a run with the free-text default "Earn the Boulder Badge."
// earned the badge, after which Offer correctly withheld the gym challenge
// (a beaten leader will not rebattle) — and the run had no stop condition,
// so it ping-ponged Pewter City <-> Pewter Gym from round 75 to the round
// cap, ~1400 prompt tokens a round, achieving nothing. Prose cannot end a
// run; only a predicate over the observation can.
func TestDefaultGoalStopsDeterministically(t *testing.T) {
	g, structured, err := agent.PlannerGoal(defaultGoal)
	if err != nil {
		t.Fatalf("default goal %q does not parse: %v", defaultGoal, err)
	}
	if !structured {
		t.Fatalf("default goal %q is prompt-only prose; a run using it can never stop on success", defaultGoal)
	}

	// Reachable: the predicate must fire from the event spelling Observe
	// actually writes, or the default is prose with extra steps.
	done := agent.Observation{Events: []string{state.EventBeatChampionRival.String()}}
	if status := agent.EvaluateGoal(g, done); !status.Complete {
		t.Fatalf("default goal not complete once the Champion is beaten: %+v", status)
	}
	// And it must not complete early: eight badges is not a finished game.
	eight := agent.Observation{Badges: []string{"Boulder", "Cascade", "Thunder", "Rainbow", "Soul", "Marsh", "Volcano", "Earth"}}
	if status := agent.EvaluateGoal(g, eight); status.Complete {
		t.Fatalf("default goal complete on badges alone: %+v", status)
	}
	// While incomplete it must still report graded progress: that summary is
	// the per-round system note, and an empty one teaches the model nothing.
	if status := agent.EvaluateGoal(g, eight); status.Summary == "" {
		t.Fatal("default goal reports no progress summary while incomplete")
	}
}
