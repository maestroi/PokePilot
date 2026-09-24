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
	m             *emu.Emu
	romData       []byte
	routePriority RoutePriority
	// battleTurns, when set, is told about every move turn skill.Battle
	// presses while this adapter executes an objective.
	battleTurns BattleTurnObserver
	// gameID is set only on instances bound at registration. Execution helpers
	// build from an emulator and ROM and never need it: the caller already
	// selected this adapter through the per-game registry lookup.
	gameID gameruntime.GameID
}

func newRedObjectiveAdapter(m *emu.Emu, romData []byte) *redObjectiveAdapter {
	return newRedObjectiveAdapterWithRoutePriority(m, romData, RoutePriorityConservative)
}

func newRedObjectiveAdapterWithRoutePriority(m *emu.Emu, romData []byte, priority RoutePriority) *redObjectiveAdapter {
	return &redObjectiveAdapter{m: m, romData: romData, routePriority: priority}
}

func init() {
	for _, id := range gen1Games {
		gameID := id
		registerObjectiveAdapterFactory(gameID, func(m *emu.Emu, romData []byte, priority RoutePriority) ObjectiveGameAdapter {
			adapter := newRedObjectiveAdapterWithRoutePriority(m, romData, priority)
			adapter.gameID = gameID
			return adapter
		})
	}
}

func redTravelCostPolicy(priority RoutePriority) skill.TravelCostPolicy {
	if priority == RoutePriorityFastest {
		return skill.TravelCostFastest
	}
	return skill.TravelCostConservative
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
			center, centerErr := skill.PokemonCenterMap(a.romData, d.Map)
			if centerErr != nil {
				// Keep validation usable with synthetic/minimal ROM fixtures. Real
				// runs use the service role; the legacy map-name check is only the
				// fail-open compatibility path when ROM role decoding is unavailable.
				center = isCenter(state.MapName(d.Map))
			}
			if !center {
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
	if missing := redMissingProgressionPrerequisites(o, obs); len(missing) != 0 {
		return progressionPrerequisiteError(missing)
	}
	if missing := redMissingFieldCapabilityPrerequisites(o, obs); len(missing) != 0 {
		return fieldCapabilityPrerequisiteError(missing)
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

// ObserveBattleTurns implements BattleTurnObservingAdapter.
func (a *redObjectiveAdapter) ObserveBattleTurns(observer BattleTurnObserver) {
	a.battleTurns = observer
}

func (a *redObjectiveAdapter) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	restoreMoveObserver := skill.WithMoveObserver(a.m, gen1MoveObserver(a.romData, a.battleTurns))
	defer restoreMoveObserver()
	result, err := executeRedOwned(a.m, a.romData, o, a.routePriority)
	return normalizeRedOwnedExecutionResult(o, result, err)
}

// normalizeRedOwnedExecutionResult gives validated, bounded owned actions a
// portable fallback outcome when their native controller path returns an
// untyped error. UseFieldItem/TeachTMHM, Buy, and Catch can all fail after
// bounded work even though their owning controller has already returned the
// game to a safe boundary. Marking that owned action blocked prevents a clean
// controller/acquisition miss from becoming terminal unknown_failure. Typed
// failures still win in NormalizeFailure, and an unsafe finish boundary or
// unreadable final observation still overrides this fallback in the transaction
// runtime.
func normalizeRedOwnedExecutionResult(o Objective, result ObjectiveResult, err error) (ObjectiveResult, error) {
	if err != nil {
		var required *skill.RequiredBattleError
		if errors.As(err, &required) {
			result.Battle = requiredBattleEvidenceFromRed(required.Outcome.Encounter, required.Outcome.Result)
			result.Outcome = OutcomeBlocked
		}
		if (o.Kind == KindUseItem || o.Kind == KindBuy || o.Kind == KindCatch) && result.Outcome == "" {
			result.Outcome = OutcomeBlocked
		}
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
	if o.Kind == KindGoTo {
		dest, ok := skill.Place(o.Place)
		if !ok {
			return fmt.Errorf("%w: destination %q no longer resolves", ErrObjectivePostconditionFailed, o.Place)
		}
		if dest.Reached(final.Map, final.X, final.Y) {
			return nil
		}
		var mem state.Mem
		state.Snapshot(a.m, &mem)
		if redOccupiedDestinationArrival(final, dest, state.DecodeSprites(&mem)) {
			return nil
		}
		return fmt.Errorf(
			"%w: %s ended on map %02x at (%d,%d), want %s destination on map %02x",
			ErrObjectivePostconditionFailed, o, final.Map, final.X, final.Y, dest.KindName(), dest.Map)
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
