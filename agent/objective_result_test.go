package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

func TestActionForObjectiveOutcome(t *testing.T) {
	cases := []struct {
		out  Outcome
		want runAction
	}{
		{OutcomeCompleted, actionContinue},
		{OutcomeBlocked, actionReplan},
		{OutcomeChoiceRequired, actionChoice},
		{OutcomeStabilizationFailed, actionStop},
		{OutcomeOwnershipFailure, actionStop},
		{OutcomeControllerUncertain, actionStop},
		{OutcomePostconditionFailed, actionStop},
		{OutcomePostconditionUnavailable, actionStop},
		{OutcomeUnknownFailure, actionStop},
		{Outcome("future-value"), actionStop},
	}
	for _, tc := range cases {
		if got := actionFor(tc.out); got != tc.want {
			t.Errorf("actionFor(%q) = %d, want %d", tc.out, got, tc.want)
		}
	}
}

func TestClassifyObjectiveOutcomePrecedence(t *testing.T) {
	clean := Observation{Controllable: true}

	lastLeg := fmt.Errorf("north edge: %w", skill.ErrLegUnwalkable)
	replan := fmt.Errorf("%w: %w", skill.ErrReplanExhausted, lastLeg)
	if got := classifyObjectiveOutcome(replan, clean); got != OutcomeControllerUncertain {
		t.Fatalf("replan exhaustion = %q, want controller_uncertain", got)
	}

	joinedChoice := errors.Join(world.ErrNoPath, ErrObjectiveBoundaryChoice)
	if got := classifyObjectiveOutcome(joinedChoice, clean); got != OutcomeChoiceRequired {
		t.Fatalf("joined choice = %q, want choice_required", got)
	}

	if got := classifyObjectiveOutcome(skill.ErrBattleInterrupted, clean); got != OutcomeOwnershipFailure {
		t.Fatalf("raw battle interruption = %q, want ownership_failure", got)
	}
	if got := classifyObjectiveOutcome(skill.ErrMenuStuck, clean); got != OutcomeControllerUncertain {
		t.Fatalf("menu stuck = %q, want controller_uncertain", got)
	}
	if got := classifyObjectiveOutcome(emu.ErrFrameDeadline, clean); got != OutcomeControllerUncertain {
		t.Fatalf("frame deadline = %q, want controller_uncertain", got)
	}
}

func TestClassifyObjectiveOutcomeKnownBlockageRequiresStableOverworld(t *testing.T) {
	if got := classifyObjectiveOutcome(world.ErrNoPath, Observation{Controllable: true}); got != OutcomeBlocked {
		t.Fatalf("clean no-path = %q, want blocked", got)
	}
	if got := classifyObjectiveOutcome(world.ErrNoPath, Observation{Controllable: false}); got != OutcomeStabilizationFailed {
		t.Fatalf("dirty no-path = %q, want stabilization_failed", got)
	}
	if got := classifyObjectiveOutcome(world.ErrNoPath, Observation{Controllable: true, InBattle: true}); got != OutcomeStabilizationFailed {
		t.Fatalf("battle no-path = %q, want stabilization_failed", got)
	}
}

func TestClassifyObjectiveOutcomeGameplayRecovery(t *testing.T) {
	clean := Observation{Controllable: true}
	for _, err := range []error{skill.ErrBlackedOut, skill.ErrTrainRetreat, skill.ErrTrainProgress} {
		if got := classifyObjectiveOutcome(err, clean); got != OutcomeBlocked {
			t.Errorf("%v = %q, want blocked", err, got)
		}
	}
}

func TestClassifyObjectiveOutcomeUnknownIsTerminal(t *testing.T) {
	if got := classifyObjectiveOutcome(errors.New("surprise"), Observation{Controllable: true}); got != OutcomeUnknownFailure {
		t.Fatalf("unknown error = %q, want unknown_failure", got)
	}
	if actionFor(OutcomeUnknownFailure) != actionStop {
		t.Fatal("unknown failure must stop by default")
	}
}

func TestObjectiveResultHistoryText(t *testing.T) {
	if got := (ObjectiveResult{Outcome: OutcomeCompleted}).HistoryText(); got != "done" {
		t.Fatalf("completed history = %q, want done", got)
	}
	got := (ObjectiveResult{Outcome: OutcomeBlocked, Summary: "blocked at Route 2"}).HistoryText()
	if got != "blocked: blocked at Route 2" {
		t.Fatalf("blocked history = %q", got)
	}
}
