package agent

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
)

func TestNormalizeRedFailureRepresentativeClasses(t *testing.T) {
	stable := Observation{Controllable: true}
	cases := []struct {
		name        string
		phase       gameruntime.FailurePhase
		err         error
		final       Observation
		class       gameruntime.FailureClass
		cause       string
		recoverable bool
	}{
		{
			name: "navigation stall", phase: gameruntime.FailurePhaseExecution,
			err: skill.ErrNavigationStalled, final: stable,
			class: gameruntime.FailureClassBlocked, cause: "navigation_stalled", recoverable: true,
		},
		{
			name: "unsafe navigation stall", phase: gameruntime.FailurePhaseExecution,
			err: skill.ErrNavigationStalled, final: Observation{},
			class: gameruntime.FailureClassControllerUncertain, cause: "navigation_stalled", recoverable: false,
		},
		{
			name: "dirty finish dominates", phase: gameruntime.FailurePhaseFinishBoundary,
			err: errors.Join(skill.ErrMenuStuck, ErrObjectiveBoundaryDirty), final: stable,
			class: gameruntime.FailureClassStabilizationFailed, cause: "objective_boundary_dirty", recoverable: false,
		},
		{
			name: "blackout", phase: gameruntime.FailurePhaseExecution,
			err: skill.ErrBlackedOut, final: stable,
			class: gameruntime.FailureClassBlocked, cause: "blacked_out", recoverable: true,
		},
		{
			name: "ownership escape", phase: gameruntime.FailurePhaseExecution,
			err: skill.ErrBattleInterrupted, final: stable,
			class: gameruntime.FailureClassOwnershipFailure, cause: "battle_escaped_owner", recoverable: false,
		},
		{
			name: "postcondition", phase: gameruntime.FailurePhasePostcondition,
			err: ErrObjectivePostconditionFailed, final: stable,
			class: gameruntime.FailureClassPostconditionFailed, cause: "objective_postcondition_failed", recoverable: true,
		},
		{
			name: "observation phase always uncertain", phase: gameruntime.FailurePhaseFinalObservation,
			err: skill.ErrNavigationStalled, final: stable,
			class: gameruntime.FailureClassControllerUncertain, cause: "navigation_stalled", recoverable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeRedFailure(tc.phase, tc.err, tc.final)
			if got.Phase != tc.phase || got.Class != tc.class || got.Cause != tc.cause || got.Recoverable != tc.recoverable {
				t.Fatalf("failure = %+v; want phase=%q class=%q cause=%q recoverable=%v", got, tc.phase, tc.class, tc.cause, tc.recoverable)
			}
		})
	}
}

func TestNormalizedFailureFingerprintIgnoresNativeErrorProse(t *testing.T) {
	obj := Objective{Kind: KindGoTo, Place: "route 1"}
	final := Observation{Location: "viridian city", X: 1, Y: 2, Controllable: true}
	failure := gameruntime.Failure{
		Phase: gameruntime.FailurePhaseExecution, Class: gameruntime.FailureClassBlocked,
		Cause: "navigation_stalled", Recoverable: true,
	}
	a := ObjectiveResult{Objective: obj, Outcome: OutcomeBlocked, Failure: &failure, Final: final, Summary: "native wording one"}
	b := a
	b.Summary = "completely different native wording"
	if recoverableFailureKey(obj, a) != recoverableFailureKey(obj, b) {
		t.Fatal("failure fingerprint changed with diagnostic prose")
	}
}

func TestNormalizedFailureFingerprintIncludesPhaseAndContext(t *testing.T) {
	obj := Objective{Kind: KindGoTo, Place: "route 1"}
	final := Observation{Location: "viridian city", X: 1, Y: 2, Controllable: true}
	first := gameruntime.Failure{
		Phase: gameruntime.FailurePhaseExecution, Class: gameruntime.FailureClassBlocked,
		Cause: "route_prerequisite_missing", Recoverable: true, Context: []string{"cut"},
	}
	second := first
	second.Phase = gameruntime.FailurePhaseFinishBoundary
	if recoverableFailureKey(obj, ObjectiveResult{Outcome: OutcomeBlocked, Failure: &first, Final: final}) ==
		recoverableFailureKey(obj, ObjectiveResult{Outcome: OutcomeBlocked, Failure: &second, Final: final}) {
		t.Fatal("failure phase did not affect fingerprint")
	}
	second = first
	second.Context = []string{"surf"}
	if recoverableFailureKey(obj, ObjectiveResult{Outcome: OutcomeBlocked, Failure: &first, Final: final}) ==
		recoverableFailureKey(obj, ObjectiveResult{Outcome: OutcomeBlocked, Failure: &second, Final: final}) {
		t.Fatal("failure context did not affect fingerprint")
	}
}
