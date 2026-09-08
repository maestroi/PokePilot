package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

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

type ObjectiveResult struct {
	Objective    Objective           `json:"objective"`
	Outcome      Outcome             `json:"outcome"`
	Summary      string              `json:"summary,omitempty"`
	Cause        FailureCauseID      `json:"cause,omitempty"`
	CauseContext []string            `json:"cause_context,omitempty"`
	Initial      *FailureState       `json:"initial,omitempty"`
	Final        Observation         `json:"final"`
	Travel       *skill.TravelResult `json:"travel,omitempty"`
	Train        *skill.TrainResult  `json:"train,omitempty"`
	GymOutcome   *state.BattleResult `json:"gym_outcome,omitempty"`
	Recovered    bool                `json:"recovered,omitempty"`
	Terminal     bool                `json:"terminal,omitempty"`
}

var (
	ErrObjectiveBoundaryChoice = errors.New("unanswered choice remains open")
	ErrObjectiveBoundaryDirty  = errors.New("objective boundary did not stabilize")

	ErrObjectivePostconditionFailed      = errors.New("objective postcondition failed")
	ErrObjectivePostconditionUnavailable = errors.New("objective postcondition unavailable")
)

type runAction uint8

const (
	actionContinue runAction = iota
	actionReplan
	actionChoice
	actionStop
)

func actionFor(out Outcome) runAction {
	switch out {
	case OutcomeCompleted:
		return actionContinue
	case OutcomeBlocked, OutcomePostconditionFailed:
		return actionReplan
	case OutcomeChoiceRequired:
		return actionChoice
	default:
		return actionStop
	}
}

func executeObjectiveResult(m *emu.Emu, romData []byte, o Objective) (ObjectiveResult, error) {
	return executeObjective(m, romData, o)
}

func finalizeObjectiveResult(o Objective, result ObjectiveResult, final Observation, err error) ObjectiveResult {
	result.Objective = o
	result.Final = final
	if err == nil {
		if result.Outcome == "" {
			result.Outcome = OutcomeCompleted
		}
	} else if result.Outcome == "" || result.Outcome == OutcomeCompleted {
		result.Outcome = classifyObjectiveOutcome(o, err, final)
	}
	if err != nil {
		result.Cause, result.CauseContext = failureCauseFor(err)
	} else if result.Outcome != OutcomeCompleted && result.Cause == "" {
		// A skill may return an explicit semantic outcome without a low-level
		// error. Keep it distinguishable without inventing error prose.
		result.Cause = FailureCauseID("outcome:" + string(result.Outcome))
	}
	result.Summary = outcomeSummary(o, result.Outcome, final, err)
	return result
}

func objectivePostcondition(o Objective, final Observation) (Outcome, error) {
	switch o.Kind {
	case KindProgress:
		if !final.Controllable || final.InBattle {
			return OutcomePostconditionUnavailable, fmt.Errorf(
				"%w: %s ended at %s (%d,%d), controllable=%v inBattle=%v",
				ErrObjectivePostconditionUnavailable, o, final.Location, final.X, final.Y, final.Controllable, final.InBattle)
		}
		if !final.Story.Has(o.Progress) {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s finished but progression fact %q is false",
				ErrObjectivePostconditionFailed, o, o.Progress)
		}
		return OutcomeCompleted, nil
	case KindGoTo:
		if !final.Controllable || final.InBattle {
			return OutcomePostconditionUnavailable, fmt.Errorf(
				"%w: %s ended on map %02x at (%d,%d), controllable=%v inBattle=%v",
				ErrObjectivePostconditionUnavailable, o, final.Map, final.X, final.Y, final.Controllable, final.InBattle)
		}
		dest, ok := skill.Place(string(o.Place))
		if !ok {
			return OutcomePostconditionFailed, fmt.Errorf("%w: destination %q no longer resolves", ErrObjectivePostconditionFailed, o.Place)
		}
		if final.Map != dest.Map || final.X != dest.X || final.Y != dest.Y {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s ended on map %02x at (%d,%d), want map %02x at (%d,%d)",
				ErrObjectivePostconditionFailed, o, final.Map, final.X, final.Y, dest.Map, dest.X, dest.Y)
		}
		return OutcomeCompleted, nil
	default:
		return OutcomeCompleted, nil
	}
}

func classifyObjectiveOutcome(_ Objective, err error, final Observation) Outcome {
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

	// A dirty finish dominates the error that caused it. errors.Join keeps the
	// original typed controller fault too, so this check must precede every
	// recoverable class or an unsafe boundary could be mislabeled blocked.
	if errors.Is(err, ErrObjectiveBoundaryDirty) || errors.Is(err, skill.ErrShopStabilization) {
		return OutcomeStabilizationFailed
	}

	if errors.Is(err, ErrObjectivePostconditionFailed) {
		return OutcomePostconditionFailed
	}
	if errors.Is(err, ErrObjectivePostconditionUnavailable) {
		return OutcomePostconditionUnavailable
	}

	if recoverableControllerFault(err) {
		if stableObjectiveBoundary(final) {
			return OutcomeBlocked
		}
		return OutcomeControllerUncertain
	}

	if errors.Is(err, skill.ErrBattle) || errors.Is(err, skill.ErrBattleInterrupted) {
		return OutcomeOwnershipFailure
	}

	if errors.Is(err, skill.ErrFieldItemNoEffect) {
		return OutcomePostconditionFailed
	}

	if errors.Is(err, skill.ErrBlackedOut) ||
		errors.Is(err, skill.ErrCatchBlackout) ||
		errors.Is(err, skill.ErrCatchHuntExhausted) ||
		errors.Is(err, skill.ErrTrainRetreat) ||
		errors.Is(err, skill.ErrTrainProgress) ||
		errors.Is(err, ErrTrainingInefficient) ||
		errors.Is(err, skill.ErrCantAfford) ||
		errors.Is(err, skill.ErrNotInStock) ||
		errors.Is(err, skill.ErrBagNotRisen) {
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
		if stableObjectiveBoundary(final) {
			return OutcomeBlocked
		}
		return OutcomeStabilizationFailed
	}

	return OutcomeUnknownFailure
}

func recoverableControllerFault(err error) bool {
	return errors.Is(err, emu.ErrFrameDeadline) ||
		errors.Is(err, skill.ErrNavigationStalled) ||
		errors.Is(err, skill.ErrReplanExhausted) ||
		errors.Is(err, skill.ErrMenuStuck) ||
		errors.Is(err, skill.ErrCutsceneTimeout) ||
		errors.Is(err, skill.ErrForcedChoiceStuck) ||
		errors.Is(err, skill.ErrPickupMenu) ||
		errors.Is(err, skill.ErrShopMenuTimeout) ||
		errors.Is(err, skill.ErrShopControllerStalled)
}

func stableObjectiveBoundary(final Observation) bool {
	return final.Controllable && !final.InBattle
}

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
	s = strings.Join(strings.Fields(s), " ")
	const max = 320
	if len(s) > max {
		s = s[:max-3] + "..."
	}
	return s
}
