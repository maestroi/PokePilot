package agent

import (
	"errors"
	"fmt"
	"strings"

	gameruntime "github.com/maestroi/pokepilot/game"
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

// EmergencyEgressEvidence is portable evidence that navigation had to leave
// the current area to recover. Detail keeps the controller's diagnostic trace
// as opaque evidence while Cause/Method remain stable aggregation fields.
type EmergencyEgressEvidence struct {
	Cause  string `json:"cause,omitempty"`
	Method string `json:"method,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// TravelEvidence is the portable semantic subset of a game adapter's travel
// result. Adapter-native controller types stay behind the adapter seam, while
// emergency recovery evidence survives so successful runs cannot hide stalls.
type TravelEvidence struct {
	Battles           int                       `json:"battles,omitempty"`
	Flees             int                       `json:"flees,omitempty"`
	Dialogues         int                       `json:"dialogues,omitempty"`
	BlackedOut        bool                      `json:"blacked_out,omitempty"`
	TrainerDefeat     bool                      `json:"trainer_defeat,omitempty"`
	Replans           int                       `json:"replans,omitempty"`
	EmergencyEgresses []EmergencyEgressEvidence `json:"emergency_egresses,omitempty"`
}

// TrainingEvidence is the portable semantic summary of one training session.
type TrainingEvidence struct {
	StartLevel int    `json:"start_level,omitempty"`
	EndLevel   int    `json:"end_level,omitempty"`
	Battles    int    `json:"battles,omitempty"`
	BlackedOut bool   `json:"blacked_out,omitempty"`
	Reached    bool   `json:"reached,omitempty"`
	Retreated  bool   `json:"retreated,omitempty"`
	Method     string `json:"method,omitempty"`
}

// BattleEvidence avoids exposing a concrete game's battle enum through the
// portable objective result while retaining positive win evidence.
type BattleEvidence struct {
	// Encounter is an adapter-owned stable identity for the required fight.
	// Generic recovery treats it as opaque semantic evidence.
	Encounter string `json:"encounter,omitempty"`
	Result    string `json:"result,omitempty"`
	Won       bool   `json:"won,omitempty"`
}

type ObjectiveResult struct {
	Objective Objective            `json:"objective"`
	Outcome   Outcome              `json:"outcome"`
	Summary   string               `json:"summary,omitempty"`
	Failure   *gameruntime.Failure `json:"failure,omitempty"`
	// Cause and CauseContext are retained as backwards-compatible flattened
	// mirrors of Failure for existing farm/repro consumers.
	Cause        FailureCauseID    `json:"cause,omitempty"`
	CauseContext []string          `json:"cause_context,omitempty"`
	Initial      *FailureState     `json:"initial,omitempty"`
	Final        Observation       `json:"final"`
	Travel       *TravelEvidence   `json:"travel,omitempty"`
	Train        *TrainingEvidence `json:"train,omitempty"`
	Battle       *BattleEvidence   `json:"gym_outcome,omitempty"`
	// InteractionPresses is positive evidence that a talk objective actually
	// opened and paged dialogue. Zero is not success evidence.
	InteractionPresses int `json:"interaction_presses,omitempty"`
	// ItemEffectVerified is set only after a field-item/TM/HM/evolution helper
	// has independently verified the requested effect. This lets postcondition
	// policy distinguish semantic evidence from a bare nil executor error.
	ItemEffectVerified bool `json:"item_effect_verified,omitempty"`
	Recovered          bool `json:"recovered,omitempty"`
	Terminal           bool `json:"terminal,omitempty"`
}

var (
	ErrObjectiveBoundaryChoice = errors.New("unanswered choice remains open")
	ErrObjectiveBoundaryDirty  = errors.New("objective boundary did not stabilize")

	ErrObjectivePostconditionFailed      = errors.New("objective postcondition failed")
	ErrObjectivePostconditionUnavailable = errors.New("objective postcondition unavailable")
	// ErrProgressVerifierMissing marks a progression objective whose ID has no
	// adapter-projected Story fact. It is always wrapped together with
	// ErrObjectivePostconditionUnavailable: a missing verifier is a registry
	// defect, so it stops the run instead of replanning a "false" fact forever.
	ErrProgressVerifierMissing = errors.New("progression goal has no registered verifier")
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

func finalizeObjectiveResult(o Objective, result ObjectiveResult, final Observation, err error) ObjectiveResult {
	result.Objective = o
	result.Final = final
	if err == nil {
		if result.Outcome == "" {
			result.Outcome = OutcomeCompleted
		}
	} else {
		if result.Failure == nil {
			out := result.Outcome
			if out == "" || out == OutcomeCompleted {
				out = OutcomeUnknownFailure
			}
			failure := gameruntime.Failure{
				Class:       failureClassForOutcome(out),
				Cause:       "unclassified_error",
				Recoverable: actionFor(out) == actionReplan,
			}
			result.Failure = &failure
		}
		if result.Outcome == "" || result.Outcome == OutcomeCompleted {
			result.Outcome = outcomeForFailureClass(result.Failure.Class)
		}
		result.Cause = FailureCauseID(result.Failure.Cause)
		result.CauseContext = append([]string(nil), result.Failure.Context...)
	}
	if err == nil && result.Outcome != OutcomeCompleted && result.Cause == "" {
		// A game adapter may return an explicit semantic outcome without a
		// low-level error. Keep it distinguishable without inventing prose.
		result.Cause = FailureCauseID("outcome:" + string(result.Outcome))
	}
	result.Summary = outcomeSummary(o, result.Outcome, final, err)
	return result
}

// objectivePostcondition is retained as the compact final-state helper used by
// focused navigation/progression/catch tests. Runtime verification uses
// verifyObjectivePostcondition so before/after deltas and structured execution
// evidence are available for every executable kind.
func objectivePostcondition(o Objective, final Observation) (Outcome, error) {
	return verifyObjectivePostcondition(o, Observation{}, final, ObjectiveResult{})
}

// verifyObjectivePostcondition positively proves success for every executable
// objective kind. The default is deliberately fail-closed: adding a new kind
// without a verifier can never inherit success from a nil executor error.
func verifyObjectivePostcondition(o Objective, initial, final Observation, result ObjectiveResult) (Outcome, error) {
	if !stableObjectiveBoundary(final) {
		return OutcomePostconditionUnavailable, fmt.Errorf(
			"%w: %s ended at %s (%d,%d), controllable=%v inBattle=%v",
			ErrObjectivePostconditionUnavailable, o, final.Location, final.X, final.Y, final.Controllable, final.InBattle)
	}

	switch o.Kind {
	case KindProgress:
		fact, ok := final.Story.Lookup(o.Progress)
		if !ok {
			return OutcomePostconditionUnavailable, fmt.Errorf(
				"%w: %w: %s", ErrObjectivePostconditionUnavailable, ErrProgressVerifierMissing, o)
		}
		if !fact.Complete {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s finished but progression fact %q is false",
				ErrObjectivePostconditionFailed, o, o.Progress)
		}
		return OutcomeCompleted, nil

	case KindRepairFieldCapability:
		for _, capability := range final.FieldCapabilities {
			if capability.Name == o.FieldCapability && capability.Usable {
				return OutcomeCompleted, nil
			}
		}
		return OutcomePostconditionFailed, fmt.Errorf(
			"%w: %s finished but field capability %q is not usable",
			ErrObjectivePostconditionFailed, o, o.FieldCapability)

	case KindGoTo:
		// Generic execution can verify a semantic location without resolving
		// native map ids or game-owned destination geometry. Concrete adapters
		// may provide a stronger exact-tile verifier before falling back here.
		if o.Place != "" && final.Location == o.Place {
			return OutcomeCompleted, nil
		}
		return OutcomePostconditionFailed, fmt.Errorf(
			"%w: %s ended at semantic location %q, want %q",
			ErrObjectivePostconditionFailed, o, final.Location, o.Place)

	case KindTalk:
		if result.InteractionPresses <= 0 {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s returned without evidence that dialogue opened and was paged",
				ErrObjectivePostconditionFailed, o)
		}
		return OutcomeCompleted, nil

	case KindTrainer:
		for _, object := range final.MapObjects {
			if object.X == o.X && object.Y == o.Y && object.Kind == "trainer" && object.Defeated {
				return OutcomeCompleted, nil
			}
		}
		return OutcomePostconditionFailed, fmt.Errorf(
			"%w: %s finished but trainer (%d,%d) is not observably defeated",
			ErrObjectivePostconditionFailed, o, o.X, o.Y)

	case KindStarter:
		want := SpeciesID(starterName(o.Starter))
		for _, mon := range final.Party {
			if mon.Species == want {
				return OutcomeCompleted, nil
			}
		}
		return OutcomePostconditionFailed, fmt.Errorf(
			"%w: %s finished but party does not contain starter %s",
			ErrObjectivePostconditionFailed, o, want)

	case KindTrain:
		if o.Intent == "dex-evolution" {
			if o.Slot < 0 || o.Slot >= len(final.Party) {
				return OutcomePostconditionFailed, fmt.Errorf(
					"%w: %s target slot %d is absent from final party of %d",
					ErrObjectivePostconditionFailed, o, o.Slot, len(final.Party))
			}
			if final.Party[o.Slot].Level < o.Level {
				return OutcomePostconditionFailed, fmt.Errorf(
					"%w: %s finished at level %d in slot %d, want at least %d",
					ErrObjectivePostconditionFailed, o, final.Party[o.Slot].Level, o.Slot, o.Level)
			}
			return OutcomeCompleted, nil
		}
		if o.Species != "" {
			for _, mon := range final.Party {
				if mon.Species == o.Species && mon.Level >= o.Level {
					return OutcomeCompleted, nil
				}
			}
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s finished but %s is not in the party at level %d+",
				ErrObjectivePostconditionFailed, o, o.Species, o.Level)
		}
		if o.Slot < 0 || o.Slot >= len(final.Party) || final.Party[o.Slot].Level < o.Level {
			got := uint8(0)
			if o.Slot >= 0 && o.Slot < len(final.Party) {
				got = final.Party[o.Slot].Level
			}
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s finished with slot %d at level %d, want at least %d",
				ErrObjectivePostconditionFailed, o, o.Slot, got, o.Level)
		}
		return OutcomeCompleted, nil

	case KindHeal:
		if len(final.Party) == 0 {
			return OutcomePostconditionFailed, fmt.Errorf("%w: %s finished with an empty party", ErrObjectivePostconditionFailed, o)
		}
		for i, mon := range final.Party {
			if mon.MaxHP == 0 || mon.HP != mon.MaxHP || mon.Status != "" {
				return OutcomePostconditionFailed, fmt.Errorf(
					"%w: %s left party slot %d at %d/%d HP status=%q",
					ErrObjectivePostconditionFailed, o, i, mon.HP, mon.MaxHP, mon.Status)
			}
		}
		return OutcomeCompleted, nil

	case KindGym:
		if result.Battle == nil || !result.Battle.Won {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s has no verified winning battle result", ErrObjectivePostconditionFailed, o)
		}
		if len(final.Badges) <= len(initial.Badges) {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s won but badge count did not increase (%d -> %d)",
				ErrObjectivePostconditionFailed, o, len(initial.Badges), len(final.Badges))
		}
		return OutcomeCompleted, nil

	case KindCatch:
		if !pokedexOwnedSet(final)[o.Species] {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s finished but Pokédex does not own %s",
				ErrObjectivePostconditionFailed, o, o.Species)
		}
		return OutcomeCompleted, nil

	case KindPickup:
		before := bagItemQuantity(initial.Bag, o.Item)
		after := bagItemQuantity(final.Bag, o.Item)
		if after != before+1 {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s finished but bag quantity changed %d -> %d, want %d",
				ErrObjectivePostconditionFailed, o, before, after, before+1)
		}
		return OutcomeCompleted, nil

	case KindUseItem:
		if !result.ItemEffectVerified {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s returned without verified item/TM/HM effect evidence",
				ErrObjectivePostconditionFailed, o)
		}
		return OutcomeCompleted, nil

	case KindBuy:
		before := bagItemQuantity(initial.Bag, o.Item)
		after := bagItemQuantity(final.Bag, o.Item)
		want := before + o.Qty
		if after != want {
			return OutcomePostconditionFailed, fmt.Errorf(
				"%w: %s finished but bag quantity changed %d -> %d, want %d",
				ErrObjectivePostconditionFailed, o, before, after, want)
		}
		return OutcomeCompleted, nil

	default:
		return OutcomePostconditionUnavailable, fmt.Errorf(
			"%w: no positive verifier registered for executable objective kind %d",
			ErrObjectivePostconditionUnavailable, int(o.Kind))
	}
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
