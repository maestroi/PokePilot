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
		{skill.ErrCatchHuntExhausted, "catch_hunt_exhausted"},
		{skill.ErrFishingHuntExhausted, "fishing_hunt_exhausted"},
		{skill.ErrFishingNoShoreline, "fishing_no_shoreline"},
		{skill.ErrFishingNoFishHere, "fishing_no_fish_here"},
	} {
		got, _ := failureCauseFor(tc.err)
		if got != tc.want {
			t.Errorf("cause(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestFailurePolicySuppressesSameStateAndExpiresOnWorldChange(t *testing.T) {
	failed := Objective{Kind: KindGoTo, Place: "route 1"}
	other := Objective{Kind: KindGoTo, Place: "pallet town"}
	obs := Observation{Location: "viridian city", X: 10, Y: 12, Controllable: true, Money: 100}
	policy := newRunFailurePolicy(3)
	result := ObjectiveResult{Objective: failed, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: obs}
	policy.record(result)

	fingerprint := fingerprintRecoverableFailure(failed, result)
	entry := policy.quarantine[objectiveStorageKey(failed)]
	if entry.Fingerprint != fingerprint.Key {
		t.Fatalf("quarantine fingerprint = %q, retry fingerprint = %q", entry.Fingerprint, fingerprint.Key)
	}

	got := policy.filter(obs, []Objective{failed, other})
	if len(got) != 1 || got[0].Key() != other.Key() {
		t.Fatalf("same-state filter = %+v, want only %s", got, other)
	}

	changed := obs
	changed.X++
	got = policy.filter(changed, []Objective{failed, other})
	if len(got) != 2 {
		t.Fatalf("changed-state filter = %+v, want both objectives", got)
	}
}

func TestFailurePolicyFailsOpenWhenNoAlternativeExists(t *testing.T) {
	failed := Objective{Kind: KindGoTo, Place: "route 1"}
	obs := Observation{Location: "viridian city", X: 10, Y: 12, Controllable: true}
	policy := newRunFailurePolicy(3)
	policy.record(ObjectiveResult{Objective: failed, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: obs})
	got := policy.filter(obs, []Objective{failed})
	if len(got) != 1 || got[0].Key() != failed.Key() {
		t.Fatalf("single-option filter = %+v, want fail-open", got)
	}
}

func TestRecoverableFailureFingerprintChangesWithRelevantState(t *testing.T) {
	obj := Objective{Kind: KindGoTo, Place: "route 1"}
	a := ObjectiveResult{Objective: obj, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: Observation{Location: "viridian city", X: 1, Y: 2, Controllable: true, Money: 100}}
	b := a
	b.Final.X++
	if recoverableFailureKey(obj, a) == recoverableFailureKey(obj, b) {
		t.Fatal("position change did not change travel failure state fingerprint")
	}
}
