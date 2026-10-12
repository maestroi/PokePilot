package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
)

// TestCatchUnreachableGrassIsRoutePrerequisite pins #2480. Route 23's grass
// is behind Surf and the badge checks, so a Fearow catch must fail as a route
// prerequisite before any walking or hunting. An untyped catch miss was
// classified outcome:blocked and exhausted the failure-recovery budget.
func TestCatchUnreachableGrassIsRoutePrerequisite(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")
	start := m.FrameCount()
	o := Objective{Kind: KindCatch, Species: "fearow", Place: "route 23", Flee: true}

	var result ObjectiveResult
	var execErr error
	err := m.WithFrameDeadline(start+60, func() error {
		result, execErr = executeCatchObjective(m, m.ROM(), o, ObjectiveResult{})
		return execErr
	})
	if errors.Is(err, emu.ErrFrameDeadline) {
		t.Fatal("unreachable grass catch started moving; want an immediate route block")
	}
	var blocked *world.RouteBlockedError
	if !errors.As(execErr, &blocked) {
		t.Fatalf("execute err = %v, want RouteBlockedError", execErr)
	}
	if m.FrameCount() != start {
		t.Fatalf("frame count %d -> %d; habitat block must not play the game", start, m.FrameCount())
	}

	failure := normalizeRedFailure(gameruntime.FailurePhaseExecution, execErr, Observation{Controllable: true})
	if failure.Cause != "route_prerequisite_missing" {
		t.Fatalf("cause = %q, want route_prerequisite_missing", failure.Cause)
	}
	result.Objective = o
	result.Outcome = OutcomeBlocked
	result.Failure = &failure
	policy := newRunFailurePolicy(2)
	for i := 0; i < 5; i++ {
		got := policy.recoverable(o, result, true, 0)
		if got.Stop != StopUnset || !got.Recovered {
			t.Fatalf("attempt %d = %+v; route-blocked catch must not spend the failure budget", i+1, got)
		}
	}
}
