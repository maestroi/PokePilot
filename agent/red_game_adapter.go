package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

const objectiveFrameBudget uint64 = 500_000

type redObjectiveAdapter struct {
	m       *emu.Emu
	romData []byte
	// gameID is set only on instances bound at registration. Execution helpers
	// build from an emulator and ROM and never need it: the caller already
	// selected this adapter through the per-game registry lookup.
	gameID gameruntime.GameID
}

func newRedObjectiveAdapter(m *emu.Emu, romData []byte) *redObjectiveAdapter {
	return &redObjectiveAdapter{m: m, romData: romData}
}

func redFieldMoveForCapability(capability CapabilityID) (skill.FieldMove, bool) {
	switch capability {
	case "cut":
		return skill.FieldCut, true
	case "fly":
		return skill.FieldFly, true
	case "surf":
		return skill.FieldSurf, true
	case "strength":
		return skill.FieldStrength, true
	case "flash":
		return skill.FieldFlash, true
	default:
		return 0, false
	}
}

func (a *redObjectiveAdapter) Observe() (Observation, error) {
	obs, err := ObserveChecked(a.m, a.romData)
	if err != nil {
		return Observation{}, err
	}
	var mem state.Mem
	state.Snapshot(a.m, &mem)
	inv := state.DecodeInventory(&mem)
	obs.Story = redProgressStateFromRAM(&mem, inv, state.DecodeStoryFacts(&mem, inv))
	return obs, nil
}

func (a *redObjectiveAdapter) Validate(o Objective, obs Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	switch o.Kind {
	case KindGoTo:
		if _, ok := skill.Place(o.Place); !ok {
			return fmt.Errorf("agent: %s: unknown Red place %q", o, o.Place)
		}
	case KindHeal:
		if o.Place != "" {
			d, ok := skill.Place(o.Place)
			if !ok {
				return fmt.Errorf("agent: %s: unknown Red place %q", o, o.Place)
			}
			if !isCenter(state.MapName(d.Map)) {
				return fmt.Errorf("agent: %s: %q is not a Pokemon Center", o, o.Place)
			}
		}
	case KindStarter:
		if o.Starter > skill.StarterBulbasaur {
			return fmt.Errorf("agent: %s: unsupported Red starter %d", o, int(o.Starter))
		}
		if o.Species != "" {
			if _, ok := redSpeciesID(o.Species); !ok {
				return fmt.Errorf("agent: %s: unknown Red starter species %q", o, o.Species)
			}
		}
	case KindProgress:
		if !redProgressionKnown(o.Progress) {
			return fmt.Errorf("agent: %s: unknown Red progression goal %q", o, o.Progress)
		}
	case KindRepairFieldCapability:
		if _, ok := redFieldMoveForCapability(o.FieldCapability); !ok {
			return fmt.Errorf("agent: %s: unknown Red field capability %q", o, o.FieldCapability)
		}
	case KindCatch:
		if _, ok := redSpeciesID(o.Species); !ok {
			return fmt.Errorf("agent: %s: unknown Red species %q", o, o.Species)
		}
	case KindPickup:
		if _, ok := a.resolvePickupItemID(obs.Map, o); !ok {
			return fmt.Errorf("agent: %s: unknown Red item %q", o, o.Item)
		}
	case KindUseItem, KindBuy:
		if _, ok := a.resolveItemID(o.Item); !ok {
			return fmt.Errorf("agent: %s: unknown Red item %q", o, o.Item)
		}
	}
	return nil
}

func (a *redObjectiveAdapter) resolveItemID(id ItemID) (uint8, bool) {
	if raw, ok := redItemID(id); ok {
		return raw, true
	}
	var mem state.Mem
	state.Snapshot(a.m, &mem)
	inv := state.DecodeInventory(&mem)
	for _, item := range inv.Items {
		machine, err := rom.LookupTMHM(a.romData, item.ID)
		if err != nil {
			continue
		}
		if machineItemID(machine) == id {
			return item.ID, true
		}
	}
	return 0, false
}

func (a *redObjectiveAdapter) NormalizeBoundary() error {
	// Boundary cleanup stays fail-closed for arbitrary choices. The one safe
	// exception is a known route-gate prompt left behind by interrupted travel:
	// declining it with NO is reversible and does not spend money or advance
	// the prior objective. If recognized, normalize any closing text normally.
	declined, err := skill.DeclineKnownRouteGate(a.m)
	if err != nil {
		return fmt.Errorf("cancel leftover route gate: %w", err)
	}
	if declined {
		return normalizeObjectiveBoundary(a.m)
	}
	return normalizeObjectiveBoundary(a.m)
}

func (a *redObjectiveAdapter) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	result, err := executeRedOwned(a.m, a.romData, o)
	return normalizeRedOwnedExecutionResult(o, result, err)
}

// normalizeRedOwnedExecutionResult gives validated item-use actions a portable
// fallback outcome when their native controller path returns an untyped error.
// UseFieldItem and TeachTMHM contain several menu-state failures that are
// ordinary replanning conditions but cannot be distinguished by error identity
// alone. Marking the owned action blocked prevents those errors from becoming a
// terminal unknown_failure. Typed failures still win in NormalizeFailure, and
// an unsafe finish boundary or unreadable final observation still overrides
// this fallback in the transaction runtime.
func normalizeRedOwnedExecutionResult(o Objective, result ObjectiveResult, err error) (ObjectiveResult, error) {
	if err != nil && o.Kind == KindUseItem && result.Outcome == "" {
		result.Outcome = OutcomeBlocked
	}
	return result, err
}

func (a *redObjectiveAdapter) WithinObjectiveBudget(o Objective, fn func() error) error {
	deadline := a.m.FrameCount() + objectiveFrameBudget
	err := a.m.WithFrameDeadline(deadline, fn)
	if errors.Is(err, emu.ErrFrameDeadline) {
		return fmt.Errorf("agent: %s: objective frame watchdog: %w", o, err)
	}
	return err
}

func (a *redObjectiveAdapter) SettlePostcondition(o Objective) error {
	return settleObjectivePostcondition(a.m, o)
}

func (a *redObjectiveAdapter) VerifyPostcondition(o Objective, initial, final Observation, result ObjectiveResult) error {
	if o.Kind == KindStarter && o.Species != "" {
		return verifyRedStarterSpeciesPostcondition(o, final)
	}

	_, err := verifyObjectivePostcondition(o, initial, final, result)
	if err == nil && o.Kind == KindHeal {
		if ppErr := verifyRedHealPPPostcondition(initial, final); ppErr != nil {
			return ppErr
		}
	}
	if o.Kind == KindTrain && o.Intent != "dex-evolution" && errors.Is(err, ErrObjectivePostconditionFailed) &&
		redTrainingReachedThroughEvolution(o, initial, final, result) {
		return nil
	}
	if o.Kind == KindGoTo && errors.Is(err, ErrObjectivePostconditionFailed) {
		dest, ok := skill.Place(o.Place)
		if ok {
			var mem state.Mem
			state.Snapshot(a.m, &mem)
			if redOccupiedDestinationArrival(final, dest, state.DecodeSprites(&mem)) {
				return nil
			}
		}
	}
	return err
}

// verifyRedStarterSpeciesPostcondition handles patched starter experiments.
// Starter selects the physical Oak ball, while Species records the semantic
// Pokemon that the patched cartridge promises to put in the party.
func verifyRedStarterSpeciesPostcondition(o Objective, final Observation) error {
	if !stableObjectiveBoundary(final) {
		return fmt.Errorf(
			"%w: %s ended at %s (%d,%d), controllable=%v inBattle=%v",
			ErrObjectivePostconditionUnavailable, o, final.Location, final.X, final.Y, final.Controllable, final.InBattle)
	}
	for _, mon := range final.Party {
		if mon.Species == o.Species {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: %s finished but party does not contain starter %s",
		ErrObjectivePostconditionFailed, o, o.Species)
}

// verifyRedHealPPPostcondition closes the positive-evidence gap for Center
// healing. Heal objectives are offered for exhausted attacking PP even when HP
// and status are already perfect, so the runtime must prove PP became usable
// rather than accepting a nil executor error plus unchanged full HP.
func verifyRedHealPPPostcondition(initial, final Observation) error {
	if !leadOutOfPP(initial) {
		return nil
	}
	if len(final.LeadPP) == 0 || leadOutOfPP(final) {
		return fmt.Errorf("%w: heal finished but lead still has no usable PP", ErrObjectivePostconditionFailed)
	}
	return nil
}

func (a *redObjectiveAdapter) NormalizeFailure(phase gameruntime.FailurePhase, err error, final Observation) gameruntime.Failure {
	return normalizeRedFailure(phase, err, final)
}

// redTrainingReachedThroughEvolution is the Red-specific fallback for ordinary
// species-targeted training. A session may legitimately replace the target
// species with its evolution while reaching the requested level. The generic
// verifier intentionally treats that species mismatch as a failure; Red can
// prove the stronger game-specific fact from semantic training evidence plus
// the same party slot that held the requested species before execution.
func redTrainingReachedThroughEvolution(o Objective, initial, final Observation, result ObjectiveResult) bool {
	if o.Kind != KindTrain || o.Species == "" || result.Train == nil || !result.Train.Reached ||
		result.Train.EndLevel < int(o.Level) || !stableObjectiveBoundary(final) {
		return false
	}

	slot := -1
	if o.Slot >= 0 && o.Slot < len(initial.Party) && initial.Party[o.Slot].Species == o.Species {
		slot = o.Slot
	} else {
		for i, mon := range initial.Party {
			if mon.Species == o.Species {
				slot = i
				break
			}
		}
	}
	if slot < 0 || slot >= len(final.Party) {
		return false
	}
	return final.Party[slot].Level >= o.Level && final.Party[slot].Species != ""
}

func (a *redObjectiveAdapter) CaptureFailure(o Objective, err error) error {
	return captureObjectiveFailure(a.m, o, err)
}

func executeObjective(m *emu.Emu, romData []byte, o Objective) (ObjectiveResult, error) {
	return Execute(m, romData, o)
}

// redOccupiedDestinationArrival mirrors GoTo's adjacent-arrival fallback.
// Only current sprite RAM is evidence; static object homes are not occupancy.
func redOccupiedDestinationArrival(final Observation, dest skill.Destination, sprites []state.SpriteState) bool {
	if !final.Controllable || final.InBattle || final.Map != dest.Map {
		return false
	}
	dx, dy := int(final.X)-int(dest.X), int(final.Y)-int(dest.Y)
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx+dy != 1 {
		return false
	}
	for _, sprite := range sprites {
		if sprite.X == int(dest.X) && sprite.Y == int(dest.Y) {
			return true
		}
	}
	return false
}
