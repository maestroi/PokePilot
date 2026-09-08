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
	result.Outcome = classifyObjectiveOutcome(err, final)
	result.Summary = outcomeSummary(o, result.Outcome, final, err)
	return result, err
}

// classifyObjectiveOutcome has load-bearing precedence. A re-plan exhaustion
// still wraps ErrLegUnwalkable, a dirty finish can be joined with the primary
// error, and an unanswered choice is more specific than generic instability.
func classifyObjectiveOutcome(err error, final Observation) Outcome {
	if err == nil {
		return OutcomeCompleted
	}

	if errors.Is(err, ErrObjectiveBoundaryChoice) {
		return OutcomeChoiceRequired
	}
	var choice *skill.ErrDialogueChoice
	if errors.As(err, &choice) {
		return OutcomeChoiceRequired
	}
	if errors.Is(err, ErrObjectiveBoundaryDirty) {
		return OutcomeStabilizationFailed
	}

	// These are bounded controllers saying they no longer know how to drive
	// the game safely. Replanning the same objective cannot repair them.
	if errors.Is(err, emu.ErrFrameDeadline) ||
		errors.Is(err, skill.ErrReplanExhausted) ||
		errors.Is(err, skill.ErrMenuStuck) ||
		errors.Is(err, skill.ErrCutsceneTimeout) {
		return OutcomeControllerUncertain
	}

	// Travel owns battle interruptions. If one escapes the objective layer,
	// the wrong layer was used rather than the game merely blocking progress.
	if errors.Is(err, skill.ErrBattle) || errors.Is(err, skill.ErrBattleInterrupted) {
		return OutcomeOwnershipFailure
	}

	// These are explicit recoverable game outcomes. They changed the world and
	// the planner should choose again from the new state.
	if errors.Is(err, skill.ErrBlackedOut) ||
		errors.Is(err, skill.ErrTrainRetreat) ||
		errors.Is(err, skill.ErrTrainProgress) {
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
	base := fmt.Sprintf("%s at %s (%d,%d)", out, place, final.X, final.Y)
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
	const max = 320
	if len(s) > max {
		s = s[:max-3] + "..."
	}
	return s
}
