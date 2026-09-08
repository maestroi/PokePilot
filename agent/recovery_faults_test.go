package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestRecoverableControllerFaultRequiresStableBoundary(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"navigation stall", skill.ErrNavigationStalled},
		{"shop timeout", skill.ErrShopMenuTimeout},
		{"shop controller", skill.ErrShopControllerStalled},
		{"menu stuck", skill.ErrMenuStuck},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("owned action: %w", tc.err)
			if got := classifyObjectiveOutcome(Objective{}, wrapped, Observation{Controllable: true}); got != OutcomeBlocked {
				t.Fatalf("stable outcome = %q, want blocked", got)
			}
			if got := classifyObjectiveOutcome(Objective{}, wrapped, Observation{}); got != OutcomeControllerUncertain {
				t.Fatalf("unsafe outcome = %q, want controller_uncertain", got)
			}
		})
	}
}

func TestDirtyBoundaryDominatesRecoverableControllerCause(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "route 1"}
	primary := fmt.Errorf("%w: repeated state", skill.ErrNavigationStalled)
	combined := objectiveBoundaryError(o, primary, fmt.Errorf("%w: cleanup failed", ErrObjectiveBoundaryDirty))
	if !errors.Is(combined, skill.ErrNavigationStalled) || !errors.Is(combined, ErrObjectiveBoundaryDirty) {
		t.Fatalf("combined error lost identity: %v", combined)
	}
	if got := classifyObjectiveOutcome(o, combined, Observation{Controllable: true}); got != OutcomeStabilizationFailed {
		t.Fatalf("outcome = %q, want stabilization_failed", got)
	}
	cause, _ := failureCauseFor(combined)
	if cause != "objective_boundary_dirty" {
		t.Fatalf("cause = %q, want objective_boundary_dirty", cause)
	}
}

func TestShopStabilizationIsTerminalEvenIfFinalSnapshotLooksControllable(t *testing.T) {
	err := errors.Join(skill.ErrShopMenuTimeout, skill.ErrShopStabilization)
	if got := classifyObjectiveOutcome(Objective{}, err, Observation{Controllable: true}); got != OutcomeStabilizationFailed {
		t.Fatalf("outcome = %q, want stabilization_failed", got)
	}
	cause, _ := failureCauseFor(err)
	if cause != "shop_stabilization_failed" {
		t.Fatalf("cause = %q, want shop_stabilization_failed", cause)
	}
}

func TestPostconditionFailureReplansButUnknownFailureStops(t *testing.T) {
	if got := actionFor(OutcomePostconditionFailed); got != actionReplan {
		t.Fatalf("postcondition action = %d, want replan", got)
	}
	if got := actionFor(OutcomeUnknownFailure); got != actionStop {
		t.Fatalf("unknown action = %d, want stop", got)
	}
}

func TestRecoverableFailureCauseVocabulary(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want FailureCauseID
	}{
		{skill.ErrNavigationStalled, "navigation_stalled"},
		{skill.ErrShopMenuTimeout, "shop_menu_timeout"},
		{skill.ErrShopControllerStalled, "shop_controller_stalled"},
		{skill.ErrShopStabilization, "shop_stabilization_failed"},
	} {
		got, _ := failureCauseFor(tc.err)
		if got != tc.want {
			t.Errorf("cause(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestFailureQuarantineSuppressesSameStateAndExpiresOnWorldChange(t *testing.T) {
	failed := Objective{Kind: KindGoTo, Place: "route 1"}
	other := Objective{Kind: KindGoTo, Place: "pallet town"}
	obs := Observation{Location: "viridian city", X: 10, Y: 12, Controllable: true, Money: 100}
	q := newFailureQuarantine()
	q.record(ObjectiveResult{Objective: failed, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: obs})

	got := q.filter(obs, []Objective{failed, other})
	if len(got) != 1 || got[0].String() != other.String() {
		t.Fatalf("same-state filter = %+v, want only %s", got, other)
	}

	changed := obs
	changed.Money++
	got = q.filter(changed, []Objective{failed, other})
	if len(got) != 2 {
		t.Fatalf("changed-state filter = %+v, want both objectives", got)
	}
}

func TestFailureQuarantineFailsOpenWhenNoAlternativeExists(t *testing.T) {
	failed := Objective{Kind: KindGoTo, Place: "route 1"}
	obs := Observation{Location: "viridian city", X: 10, Y: 12, Controllable: true}
	q := newFailureQuarantine()
	q.record(ObjectiveResult{Objective: failed, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: obs})
	got := q.filter(obs, []Objective{failed})
	if len(got) != 1 || got[0].String() != failed.String() {
		t.Fatalf("single-option filter = %+v, want fail-open", got)
	}
}

func TestRecoverableFailureFingerprintChangesWithRelevantState(t *testing.T) {
	obj := Objective{Kind: KindGoTo, Place: "route 1"}
	a := ObjectiveResult{Objective: obj, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: Observation{Location: "viridian city", X: 1, Y: 2, Controllable: true, Money: 100}}
	b := a
	b.Final.Money = 200
	if recoverableFailureKey(obj, a) == recoverableFailureKey(obj, b) {
		t.Fatal("money change did not change failure state fingerprint")
	}
}
