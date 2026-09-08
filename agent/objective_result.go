package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// Outcome is the normalized meaning of one objective attempt. It is deliberately
// smaller than the set of raw skill errors: planners and run policy need to know
// whether an attempt completed, was blocked by the game, needs an explicit
// choice owner, or exposed a controller/ownership defect. They do not need to
// reverse-engineer that from an error string.
type Outcome string

const (
	OutcomeCompleted                Outcome = "completed"
	OutcomeBlocked                  Outcome = "blocked"
	OutcomeChoiceRequired           Outcome = "choice_required"
	OutcomeStabilizationFailed      Outcome = "stabilization_failed"
	OutcomeOwnershipFailure         Outcome = "ownership_failure"
	OutcomeControllerUncertain      Outcome = "controller_uncertain"
	OutcomePostconditionFailed      Outcome = "postcondition_failed"
	OutcomePostconditionUnavailable Outcome = "postcondition_unavailable"
	OutcomeUnknownFailure           Outcome = "unknown_failure"
)

// ObjectiveResult is the durable, planner-facing record of one objective
// transaction. Error stays a separate Go error so errors.Is/errors.As retain
// the exact low-level identity; this value contains only stable data that can be
// serialized into run/farm output.
type ObjectiveResult struct {
	Objective Objective   `json:"objective"`
	Outcome   Outcome     `json:"outcome"`
	Summary   string      `json:"summary,omitempty"`
	Final     Observation `json:"final"`
}

// Boundary cleanup may only undo semantically reversible UI state. These
// sentinels let outcome normalization distinguish an explicit unanswered choice
// from a controller that simply failed to regain a clean overworld boundary.
var (
	ErrObjectiveBoundaryChoice = errors.New("unanswered choice remains open")
	ErrObjectiveBoundaryDirty  = errors.New("objective boundary did not stabilize")
)

type runAction uint8

const (
	actionContinue runAction = iota
	actionReplan
	actionChoice
	actionStop
)

// actionFor is intentionally default-stop. A new failure label cannot silently
// become recoverable just because a future caller forgot to update this table.
func actionFor(out Outcome) runAction {
	switch out {
	case OutcomeCompleted:
		return actionContinue
	case OutcomeBlocked:
		return actionReplan
	case OutcomeChoiceRequired:
		return actionChoice
	default:
		return actionStop
	}
}

// executeObjectiveResult is the normalization boundary between the stable
// objective executor and Run. executeObjective owns start/execute/finish UI
// lifecycle; this layer assigns semantic meaning to its result exactly once.
func executeObjectiveResult(m *emu.Emu, romData []byte, o Objective) (ObjectiveResult, error) {
	err := executeObjective(m, romData, o)
	final := Observe(m, romData)
	result := ObjectiveResult{Objective: o, Final: final}
	if err == nil {
		result.Outcome = OutcomeCompleted
		result.Summary = outcomeSummary(o, result.Outcome, final, nil)
		return result, nil
	}
	result.Outcome = classifyObjectiveOutcome(o, err, final)
	result.Summary = outcomeSummary(o, result.Outcome, final, err)
	return result, err
}

// classifyObjectiveOutcome has load-bearing precedence. A route/controller
// sentinel may be joined with a dirty finish, and an unanswered choice is more
// specific than every other boundary problem. Preserve the most useful primary
// diagnosis before falling back to generic stabilization failure; only ordinary
// gameplay blockage is allowed to re-plan.
//
// The executor predates ObjectiveResult, so three normal game endings are still
// encoded as narrow, stable agent error phrases (gym loss, bounded train
// shortfall, bounded/missed catch). expectedLegacyGameplayBlockage is the one
// compatibility bridge for those; new code should expose a typed sentinel or a
// structured skill result instead of adding another phrase here.
func classifyObjectiveOutcome(o Objective, err error, final Observation) Outcome {
	if err == nil {
		return OutcomeCompleted
	}

	if errors.Is(err, ErrObjectiveBoundaryChoice) {
		return OutcomeChoiceRequired
	}
	var choice *skill.ErrDialogueChoice
	if errors.As(err, &choice) || errors.Is(err, skill.ErrFieldItemPrompt) {
		return OutcomeChoiceRequired
	}

	// These are bounded controllers saying they no longer know how to drive
	// the game safely. Replanning the same objective cannot repair them. This
	// check precedes generic boundary dirtiness because objectiveBoundaryError
	// preserves both identities with errors.Join.
	if errors.Is(err, emu.ErrFrameDeadline) ||
		errors.Is(err, skill.ErrReplanExhausted) ||
		errors.Is(err, skill.ErrMenuStuck) ||
		errors.Is(err, skill.ErrCutsceneTimeout) ||
		errors.Is(err, skill.ErrForcedChoiceStuck) ||
		errors.Is(err, skill.ErrPickupMenu) {
		return OutcomeControllerUncertain
	}

	// Travel owns battle interruptions. If one escapes the objective layer,
	// the wrong layer was used rather than the game merely blocking progress.
	if errors.Is(err, skill.ErrBattle) || errors.Is(err, skill.ErrBattleInterrupted) {
		return OutcomeOwnershipFailure
	}

	// A field item that was consumed/ran to completion without changing its
	// target violated the objective's positive effect postcondition. Retrying as
	// ordinary gameplay would conceal that invariant failure.
	if errors.Is(err, skill.ErrFieldItemNoEffect) {
		return OutcomePostconditionFailed
	}

	// If an otherwise meaningful primary error also left the emulator outside a
	// safe boundary, the dirty finish wins over anything that would normally be
	// recoverable. Run must not hand an unsettled game back to the planner.
	if errors.Is(err, ErrObjectiveBoundaryDirty) {
		return OutcomeStabilizationFailed
	}

	// Explicit game outcomes that leave (or return to) a settled world are
	// planner-visible blockage rather than controller defects.
	if errors.Is(err, skill.ErrBlackedOut) ||
		errors.Is(err, skill.ErrCatchBlackout) ||
		errors.Is(err, skill.ErrTrainRetreat) ||
		errors.Is(err, skill.ErrTrainProgress) ||
		errors.Is(err, skill.ErrCantAfford) ||
		errors.Is(err, skill.ErrNotInStock) ||
		errors.Is(err, skill.ErrBagNotRisen) ||
		expectedLegacyGameplayBlockage(o, err) {
		return OutcomeBlocked
	}

	var blocked *skill.ErrBlocked
	knownBlockage := errors.Is(err, world.ErrNoPath) ||
		errors.Is(err, world.ErrNoRoute) ||
		errors.Is(err, skill.ErrLegUnwalkable) ||
		errors.Is(err, skill.ErrNoDialogue) ||
		errors.Is(err, skill.ErrDialogueInterrupted) ||
		errors.As(err, &blocked)
	if knownBlockage {
		if final.Controllable && !final.InBattle {
			return OutcomeBlocked
		}
		return OutcomeStabilizationFailed
	}

	return OutcomeUnknownFailure
}

// expectedLegacyGameplayBlockage keeps legacy normal outcomes recoverable while
// the public Execute(error-only) contract is migrated incrementally. Each match
// is constrained by objective kind and by text owned in agent/objective.go; it
// must never become a generic substring classifier for arbitrary skill errors.
func expectedLegacyGameplayBlockage(o Objective, err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	switch o.Kind {
	case KindGym:
		return strings.Contains(s, "lost to the gym leader (blacked out to the center)")
	case KindTrain:
		return strings.Contains(s, "target level ") && strings.Contains(s, " not reached (ended level ")
	case KindCatch:
		return (strings.Contains(s, ": no ") && strings.Contains(s, " caught (outcome ")) ||
			strings.Contains(s, " encounters without a wanted species")
	}
	return false
}

// HistoryText is the compact form placed in Observation.History. Completed
// keeps the legacy "done" spelling for prompt compatibility; failures start
// with a stable outcome code so the planner no longer has to infer categories
// from arbitrary error prose.
func (r ObjectiveResult) HistoryText() string {
	if r.Outcome == OutcomeCompleted {
		return "done"
	}
	if r.Summary == "" {
		return string(r.Outcome)
	}
	return fmt.Sprintf("%s: %s", r.Outcome, r.Summary)
}

func outcomeSummary(o Objective, out Outcome, final Observation, err error) string {
	place := final.MapName
	if place == "" {
		place = fmt.Sprintf("map %02x", final.Map)
	}
	base := fmt.Sprintf("at %s (%d,%d)", place, final.X, final.Y)
	if err == nil {
		return base
	}
	detail := conciseObjectiveError(o, err)
	if detail == "" {
		return base
	}
	return base + ": " + detail
}

func conciseObjectiveError(o Objective, err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	prefix := "agent: " + o.String() + ": "
	for strings.HasPrefix(s, prefix) {
		s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
	}
	// errors.Join deliberately preserves every typed cause and renders them on
	// separate lines. History is one compact planner fact, so collapse display
	// whitespace here while leaving the original Go error untouched in Result.Err.
	s = strings.Join(strings.Fields(s), " ")
	const max = 320
	if len(s) > max {
		s = s[:max-3] + "..."
	}
	return s
}
