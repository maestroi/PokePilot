package agent

import (
	"errors"
	"fmt"
	"strings"
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
		{OutcomePostconditionFailed, actionReplan},
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
	o := Objective{Kind: KindGoTo, Place: "pewter city"}

	lastLeg := fmt.Errorf("north edge: %w", skill.ErrLegUnwalkable)
	replan := fmt.Errorf("%w: %w", skill.ErrReplanExhausted, lastLeg)
	if got := classifyObjectiveOutcome(o, replan, clean); got != OutcomeBlocked {
		t.Fatalf("replan exhaustion = %q, want blocked", got)
	}

	joinedChoice := errors.Join(world.ErrNoPath, ErrObjectiveBoundaryChoice)
	if got := classifyObjectiveOutcome(o, joinedChoice, clean); got != OutcomeChoiceRequired {
		t.Fatalf("joined choice = %q, want choice_required", got)
	}

	joinedController := errors.Join(skill.ErrMenuStuck, ErrObjectiveBoundaryDirty)
	if got := classifyObjectiveOutcome(o, joinedController, clean); got != OutcomeStabilizationFailed {
		t.Fatalf("controller + dirty boundary = %q, want stabilization_failed", got)
	}
	joinedBlocked := errors.Join(world.ErrNoPath, ErrObjectiveBoundaryDirty)
	if got := classifyObjectiveOutcome(o, joinedBlocked, clean); got != OutcomeStabilizationFailed {
		t.Fatalf("blocked + dirty boundary = %q, want stabilization_failed", got)
	}

	postFailed := fmt.Errorf("wrapped: %w", ErrObjectivePostconditionFailed)
	if got := classifyObjectiveOutcome(o, postFailed, clean); got != OutcomePostconditionFailed {
		t.Fatalf("postcondition failed = %q, want postcondition_failed", got)
	}
	postUnavailable := fmt.Errorf("wrapped: %w", ErrObjectivePostconditionUnavailable)
	if got := classifyObjectiveOutcome(o, postUnavailable, clean); got != OutcomePostconditionUnavailable {
		t.Fatalf("postcondition unavailable = %q, want postcondition_unavailable", got)
	}

	if got := classifyObjectiveOutcome(o, skill.ErrBattleInterrupted, clean); got != OutcomeOwnershipFailure {
		t.Fatalf("raw battle interruption = %q, want ownership_failure", got)
	}
	if got := classifyObjectiveOutcome(o, skill.ErrMenuStuck, clean); got != OutcomeBlocked {
		t.Fatalf("menu stuck = %q, want blocked", got)
	}
	if got := classifyObjectiveOutcome(o, emu.ErrFrameDeadline, clean); got != OutcomeBlocked {
		t.Fatalf("frame deadline = %q, want blocked", got)
	}
}

func TestClassifyObjectiveOutcomeKnownBlockageRequiresStableOverworld(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "pewter city"}
	if got := classifyObjectiveOutcome(o, world.ErrNoPath, Observation{Controllable: true}); got != OutcomeBlocked {
		t.Fatalf("clean no-path = %q, want blocked", got)
	}
	if got := classifyObjectiveOutcome(o, world.ErrNoPath, Observation{Controllable: false}); got != OutcomeStabilizationFailed {
		t.Fatalf("dirty no-path = %q, want stabilization_failed", got)
	}
	if got := classifyObjectiveOutcome(o, world.ErrNoPath, Observation{Controllable: true, InBattle: true}); got != OutcomeStabilizationFailed {
		t.Fatalf("battle no-path = %q, want stabilization_failed", got)
	}
}

func TestClassifyObjectiveOutcomeGameplayRecovery(t *testing.T) {
	clean := Observation{Controllable: true}
	o := Objective{Kind: KindGoTo, Place: "pewter city"}
	for _, err := range []error{
		skill.ErrBlackedOut,
		skill.ErrCatchBlackout,
		skill.ErrCatchHuntExhausted,
		skill.ErrTrainRetreat,
		skill.ErrTrainProgress,
		skill.ErrCantAfford,
		skill.ErrNotInStock,
		skill.ErrBagNotRisen,
	} {
		if got := classifyObjectiveOutcome(o, err, clean); got != OutcomeBlocked {
			t.Errorf("%v = %q, want blocked", err, got)
		}
	}
}

func TestClassifyObjectiveOutcomeDoesNotParseLegacyGameplayProse(t *testing.T) {
	clean := Observation{Controllable: true}
	legacy := []struct {
		o   Objective
		err error
	}{
		{
			Objective{Kind: KindGym},
			fmt.Errorf("agent: beat the gym leader here: lost to the gym leader (blacked out to the center)"),
		},
		{
			Objective{Kind: KindTrain, Level: 20},
			fmt.Errorf("agent: train the lead to level 20: target level 20 not reached (ended level 18 after 20 battles)"),
		},
		{
			Objective{Kind: KindCatch, Species: "pidgey"},
			fmt.Errorf("agent: catch a PIDGEY here: no PIDGEY caught (outcome out of balls, balls=5, encounters=1)"),
		},
		{
			Objective{Kind: KindCatch, Species: "pidgey"},
			fmt.Errorf("agent: catch a PIDGEY here: skill: Catch: 500 grass legs and 26 encounters without a wanted species (map 0x33)"),
		},
	}
	for _, tc := range legacy {
		if got := classifyObjectiveOutcome(tc.o, tc.err, clean); got != OutcomeUnknownFailure {
			t.Errorf("legacy prose %s / %v = %q, want unknown_failure", tc.o, tc.err, got)
		}
	}
}

func TestObjectivePostconditionGoToExactDestination(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "pallet town"}
	dest, ok := skill.Place(o.Place)
	if !ok {
		t.Fatal("Place(pallet town) did not resolve")
	}

	good := Observation{Map: dest.Map, X: dest.X, Y: dest.Y, Controllable: true}
	if out, err := objectivePostcondition(o, good); err != nil || out != OutcomeCompleted {
		t.Fatalf("exact destination = %q, %v; want completed, nil", out, err)
	}

	wrongTile := good
	wrongTile.X++
	out, err := objectivePostcondition(o, wrongTile)
	if out != OutcomePostconditionFailed || !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("wrong tile = %q, %v; want postcondition_failed sentinel", out, err)
	}

	unreadable := good
	unreadable.Controllable = false
	out, err = objectivePostcondition(o, unreadable)
	if out != OutcomePostconditionUnavailable || !errors.Is(err, ErrObjectivePostconditionUnavailable) {
		t.Fatalf("unreadable state = %q, %v; want postcondition_unavailable sentinel", out, err)
	}

	inBattle := good
	inBattle.InBattle = true
	out, err = objectivePostcondition(o, inBattle)
	if out != OutcomePostconditionUnavailable || !errors.Is(err, ErrObjectivePostconditionUnavailable) {
		t.Fatalf("battle state = %q, %v; want postcondition_unavailable sentinel", out, err)
	}
}

func TestObjectivePostconditionDefersToSkillForOtherKinds(t *testing.T) {
	out, err := objectivePostcondition(Objective{Kind: KindUseItem, Item: "potion"}, Observation{})
	if err != nil || out != OutcomeCompleted {
		t.Fatalf("UseItem postcondition = %q, %v; want skill-owned completed", out, err)
	}
}

func TestClassifyObjectiveOutcomePostconditionAndPrompt(t *testing.T) {
	clean := Observation{Controllable: true}
	o := Objective{Kind: KindUseItem, Item: "potion", Slot: 0}
	if got := classifyObjectiveOutcome(o, skill.ErrFieldItemNoEffect, clean); got != OutcomePostconditionFailed {
		t.Fatalf("field item no effect = %q, want postcondition_failed", got)
	}
	if got := classifyObjectiveOutcome(o, skill.ErrFieldItemPrompt, clean); got != OutcomeChoiceRequired {
		t.Fatalf("field item prompt = %q, want choice_required", got)
	}
}

func TestClassifyObjectiveOutcomeUnknownIsTerminal(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "pewter city"}
	if got := classifyObjectiveOutcome(o, errors.New("surprise"), Observation{Controllable: true}); got != OutcomeUnknownFailure {
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
	got := (ObjectiveResult{Outcome: OutcomeBlocked, Summary: "at Route 2"}).HistoryText()
	if got != "blocked: at Route 2" {
		t.Fatalf("blocked history = %q", got)
	}
}

func TestConciseObjectiveErrorIsOneLine(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "pewter city"}
	err := errors.Join(
		fmt.Errorf("agent: %s: primary failure", o),
		fmt.Errorf("objective left invalid boundary: %w", ErrObjectiveBoundaryDirty),
	)
	got := conciseObjectiveError(o, err)
	if strings.Contains(got, "\n") {
		t.Fatalf("concise error contains newline: %q", got)
	}
	if !strings.Contains(got, "primary failure") || !strings.Contains(got, "objective left invalid boundary") {
		t.Fatalf("concise error lost joined evidence: %q", got)
	}
}
