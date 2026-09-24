package agent

import (
	"errors"
	"fmt"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
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
			name: "semantic transition failure", phase: gameruntime.FailurePhaseExecution,
			err: &world.TransitionExecutionError{
				Transition: gameruntime.Transition{ID: "red:route21_surf"},
				Cause:      errors.New("live transition controller rejected action"),
			},
			final: stable,
			class: gameruntime.FailureClassBlocked, cause: "transition_execution_failed", recoverable: true,
		},
		{
			name: "unsafe semantic transition failure", phase: gameruntime.FailurePhaseExecution,
			err: &world.TransitionExecutionError{
				Transition: gameruntime.Transition{ID: "red:route21_surf"},
				Cause:      errors.New("live transition controller rejected action"),
			},
			final: Observation{},
			class: gameruntime.FailureClassControllerUncertain, cause: "transition_execution_failed", recoverable: false,
		},
		{
			name: "semantic transition stall", phase: gameruntime.FailurePhaseExecution,
			err: &world.TransitionExecutionError{
				Transition: gameruntime.Transition{ID: "red:route21_surf"},
				Cause:      world.ErrTransitionExecutionStalled,
			},
			final: stable,
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
			name: "pickup approach did not reach the item", phase: gameruntime.FailurePhaseExecution,
			err: fmt.Errorf("pickup: %w", skill.ErrPickupApproachIncomplete), final: stable,
			class: gameruntime.FailureClassUnknown, cause: "pickup_approach_incomplete", recoverable: false,
		},
		{
			name: "observation phase always uncertain", phase: gameruntime.FailurePhaseFinalObservation,
			err: skill.ErrNavigationStalled, final: stable,
			class: gameruntime.FailureClassControllerUncertain, cause: "navigation_stalled", recoverable: false,
		},
		// MEASURED run-7r4gd76w4w061ewqnfebx7pw0 round 5: "catch a TENTACOOL
		// here" exhausted Fish's rod-attempt budget (32 casts, 11 encounters,
		// never the wanted species) and fell through to unknown_failure/
		// unknown_error because ErrFishingHuntExhausted was never classified,
		// even though the grass-hunt equivalent (ErrCatchHuntExhausted) already
		// gets the ordinary bounded-hunt "blocked, replan" treatment. That
		// misclassification is what stopped the run instead of letting the
		// planner retry or pick a different objective.
		{
			name: "fishing hunt exhausted", phase: gameruntime.FailurePhaseExecution,
			err: skill.ErrFishingHuntExhausted, final: stable,
			class: gameruntime.FailureClassBlocked, cause: "fishing_hunt_exhausted", recoverable: true,
		},
		{
			name: "fishing no shoreline", phase: gameruntime.FailurePhaseExecution,
			err: skill.ErrFishingNoShoreline, final: stable,
			class: gameruntime.FailureClassBlocked, cause: "fishing_no_shoreline", recoverable: true,
		},
		{
			name: "fishing no fish here", phase: gameruntime.FailurePhaseExecution,
			err: skill.ErrFishingNoFishHere, final: stable,
			class: gameruntime.FailureClassBlocked, cause: "fishing_no_fish_here", recoverable: true,
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

// TestReplanExhaustedTakesPrecedenceOverWrappedRouteBlockedError reproduces
// MEASURED run-1ttew0yypzkgt2p7r4bm5c61ur round 6: a live NPC parked on a
// one-tile Route 13 connection band made every early re-plan fail on
// leg_unwalkable, but the LAST leg tried before the budget ran out happened
// to be a genuinely capability-gated route, wrapped verbatim into
// ErrReplanExhausted by newReplanExhaustedError (skill/goto.go). Before this
// fix, failureCauseFor's errors.As(&RouteBlockedError) matched through that
// wrapped tree first and mislabeled a transient re-plan exhaustion as
// "route_prerequisite_missing" — which then quarantined the objective under
// recoveryScopeRoutePrerequisite, a scope that only expires once the "missing"
// capability becomes true. It was never actually missing, so the quarantine
// never expired.
func TestReplanExhaustedTakesPrecedenceOverWrappedRouteBlockedError(t *testing.T) {
	routeBlocked := &world.RouteBlockedError{Blockages: []gameruntime.TransitionBlockage{
		{Missing: []gameruntime.CapabilityID{"surf"}},
	}}
	exhausted := fmt.Errorf("%w: 8 re-plans from map 18 at (11,4) toward map 05 at (11,4), last leg: %w",
		skill.ErrReplanExhausted, routeBlocked)

	cause, ctx := failureCauseFor(exhausted)
	if cause != "route_replan_exhausted" {
		t.Fatalf("cause = %q, ctx = %v; want route_replan_exhausted (got the shadowed route_prerequisite_missing)", cause, ctx)
	}
}

func TestTransitionExecutionFailurePreservesSemanticContext(t *testing.T) {
	err := &world.TransitionExecutionError{
		Transition: gameruntime.Transition{ID: "red:route21_surf"},
		Cause:      errors.New("shore rejected"),
	}
	got := normalizeRedFailure(gameruntime.FailurePhaseExecution, err, Observation{Controllable: true})
	if len(got.Context) != 1 || got.Context[0] != "red:route21_surf" {
		t.Fatalf("transition failure context = %v; want semantic transition id", got.Context)
	}
}

func TestTransitionExecutionBlockageNormalizesMissingCapability(t *testing.T) {
	err := &world.TransitionExecutionError{
		Transition: gameruntime.Transition{ID: "red:test_gate"},
		Cause: &gameruntime.TransitionBlockage{
			Transition: gameruntime.Transition{ID: "red:test_gate"},
			Missing:    []gameruntime.CapabilityID{"surf"},
		},
	}
	got := normalizeRedFailure(gameruntime.FailurePhaseExecution, err, Observation{Controllable: true})
	if got.Cause != "route_prerequisite_missing" || len(got.Context) != 1 || got.Context[0] != "surf" {
		t.Fatalf("transition blockage = %+v; want route prerequisite surf", got)
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
