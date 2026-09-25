package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestSaffronGateTypedFailuresDoNotBecomeUnknown(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		cause FailureCauseID
	}{
		{"insufficient money", skill.ErrCantAfford, "cant_afford"},
		{"interaction stalled", skill.ErrSaffronGateInteractionStalled, "saffron_gate_interaction_stalled"},
		{"drink postcondition", skill.ErrBagNotRisen, "bag_not_risen"},
		{"navigation drift", skill.ErrNavigationStalled, "navigation_stalled"},
	}
	final := Observation{Controllable: true}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.Join(errors.New("OpenSaffronGate failed"), tc.err)
			if got := classifyObjectiveOutcome(Objective{}, err, final); got == OutcomeUnknownFailure {
				t.Fatalf("classifyObjectiveOutcome(%v) = unknown_failure", tc.err)
			}
			cause, _ := failureCauseFor(err)
			if cause != tc.cause {
				t.Fatalf("failureCauseFor(%v) = %q, want %q", tc.err, cause, tc.cause)
			}
		})
	}
}
