package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
	gameruntime "github.com/maestroi/pokepilot/game"
)

// divertedTerminalRunFailure builds the #2939 farm outcome after the fix: a
// Route 16 Snorlax static capture whose boundary did not stabilize (joined
// with the typed "static capture unavailable" fault), classified terminal but
// marked recovered because the run loop diverted it into the replan flow and
// advanced to the next objective.
func divertedTerminalRunFailure() agent.ObjectiveResult {
	obj := agent.Objective{Kind: agent.KindCatch, Species: agent.SpeciesID("snorlax"), Place: "route 16"}
	return agent.ObjectiveResult{
		Objective: obj,
		Outcome:   agent.OutcomeStabilizationFailed,
		Summary:   "stabilization_failed at route 16 (12,4)",
		Failure: &gameruntime.Failure{
			Class:   gameruntime.FailureClassStabilizationFailed,
			Cause:   "objective_boundary_dirty",
			Context: []string{"primary:static_capture_unavailable"},
		},
		Cause:        "objective_boundary_dirty",
		CauseContext: []string{"primary:static_capture_unavailable"},
		Recovered:    true,
		Terminal:     false,
		Final: agent.Observation{
			Map:          0x1b,
			Location:     "route 16",
			X:            12,
			Y:            4,
			Controllable: false,
			InBattle:     true,
			GameID:       "pokemon-gold",
		},
	}
}

// TestFarmRunRecoversFromTerminalRunErrorTelemetry is the farm-level
// regression test for #2939: when a run recovers from a terminal-classified
// objective error (the diverted replan path), the finish telemetry must not
// report a terminal occurrence, so the wall does not file a
// "terminal run error stopped the run" progression blocker and the farm
// keeps leasing the next tasks.
func TestFarmRunRecoversFromTerminalRunErrorTelemetry(t *testing.T) {
	resetObjectiveFailureTelemetry()
	t.Cleanup(resetObjectiveFailureTelemetry)

	failure := divertedTerminalRunFailure()
	next := agent.ObjectiveResult{
		Objective: agent.Objective{Kind: agent.KindGoTo, Place: "route 17"},
		Outcome:   agent.OutcomeCompleted,
		Summary:   "done",
		Final: agent.Observation{
			Map:          0x1c,
			Location:     "route 17",
			Controllable: true,
			GameID:       "pokemon-gold",
		},
	}
	captureObjectiveFailureTelemetry(agent.Result{
		Stop:     agent.StopDone,
		Outcomes: []agent.ObjectiveResult{failure, next},
	})

	got, terminal := drainObjectiveFailureTelemetry("done", "build-fix", "")
	if terminal != nil {
		t.Fatalf("terminal occurrence = %+v, want nil: a recovered terminal-classified error must not stop the run report", terminal)
	}
	if len(got) != 1 {
		t.Fatalf("failure groups = %+v, want the single recovered group", got)
	}
	group := got[0]
	if group.TerminalCount != 0 || group.RecoveredCount != 1 || group.Blocking {
		t.Fatalf("group classification = terminal=%d recovered=%d blocking=%t, want 0/1/false",
			group.TerminalCount, group.RecoveredCount, group.Blocking)
	}
	if group.Recovered != true {
		t.Fatalf("group.Recovered = %t, want true", group.Recovered)
	}
}

// TestFarmRunStillReportsGenuineTerminalRunError keeps the fail-closed side:
// a genuine terminal run error (unknown controller fault, no recoverable
// primary cause) still marks the occurrence terminal and blocking, which is
// what makes the wall mark the attempt failed and retry it.
func TestFarmRunStillReportsGenuineTerminalRunError(t *testing.T) {
	resetObjectiveFailureTelemetry()
	t.Cleanup(resetObjectiveFailureTelemetry)

	failure := agent.ObjectiveResult{
		Objective: agent.Objective{Kind: agent.KindCatch, Species: agent.SpeciesID("snorlax"), Place: "route 16"},
		Outcome:   agent.OutcomeUnknownFailure,
		Summary:   "unknown_failure at route 16 (12,4)",
		Cause:     "unknown_error",
		Terminal:  true,
		Final: agent.Observation{
			Map:      0x1b,
			Location: "route 16",
			GameID:   "pokemon-gold",
		},
	}
	captureObjectiveFailureTelemetry(agent.Result{
		Stop:     agent.StopError,
		Outcomes: []agent.ObjectiveResult{failure, failure},
	})

	got, terminal := drainObjectiveFailureTelemetry("error", "build-fix", "")
	if terminal == nil {
		t.Fatalf("terminal occurrence = nil, want the genuine terminal run error to be reported")
	}
	if len(got) != 1 || !got[0].Blocking || got[0].TerminalCount != 2 {
		t.Fatalf("failure groups = %+v, want one blocking group with terminal_count=2", got)
	}
}
