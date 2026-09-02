package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
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

	if status := agent.EvaluateGoal(g, agent.Observation{Badges: []string{"Boulder"}}); !status.Complete {
		t.Fatalf("default goal not complete once the Boulder badge is held: %+v", status)
	}
	if status := agent.EvaluateGoal(g, agent.Observation{}); status.Complete {
		t.Fatal("default goal complete with no badges")
	}
}
