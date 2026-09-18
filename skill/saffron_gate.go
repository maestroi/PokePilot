package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	celadonMartRoofMap uint8 = 0x7e
	route7GateMap      uint8 = 0x4c

	freshWaterItem uint8 = 0x3c
	sodaPopItem    uint8 = 0x3d
	lemonadeItem   uint8 = 0x3e

	freshWaterPrice = 200

	vendingMachineX uint8 = 10
	vendingMachineY uint8 = 1
	vendingStandX   uint8 = 10
	vendingStandY   uint8 = 2

	route7GuardStandX   uint8 = 2
	route7GuardStandY   uint8 = 3
	route7GuardTriggerX uint8 = 3
	route7GuardTriggerY uint8 = 3

	saffronGateInteractionBudget = 8000
	saffronGateTravelBattles     = 80
)

// SaffronGateOpen is the positive story postcondition for the first phase of
// issue #34. The flag is decoded through StoryFacts so callers do not depend
// on the raw wStatusFlags1 bit used by Red's four Saffron gate scripts.
func SaffronGateOpen(mem *state.Mem) bool {
	return state.DecodeStoryFacts(mem, state.DecodeInventory(mem)).SaffronGateOpen
}

// SaffronGateReady keeps the issue #34 handoff explicit: this phase is offered
// only after issue #33's Soul Badge + Surf + Strength postcondition exists.
func SaffronGateReady(mem *state.Mem) bool {
	return state.DecodeStoryFacts(mem, state.DecodeInventory(mem)).FuchsiaProgressionComplete
}

// guardDrinkInBag reports whether Red's RemoveGuardDrink routine can consume
// one of the player's current bag entries. The order mirrors GuardDrinksList,
// but the particular drink does not matter to the gate postcondition.
func guardDrinkInBag(mem *state.Mem) (uint8, bool) {
	for _, item := range [...]uint8{freshWaterItem, sodaPopItem, lemonadeItem} {
		if _, count := bagEntry(mem, item); count > 0 {
			return item, true
		}
	}
	return 0, false
}

// OpenSaffronGate obtains a valid drink when necessary and gives it to the
// Route 7 guard. It is resumable at every durable boundary: if a checkpoint is
// taken after buying the drink, the next invocation reuses it; if the global
// Saffron guard flag is already set, it returns without moving.
func OpenSaffronGate(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: OpenSaffronGate: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if SaffronGateOpen(&mem) {
		return nil
	}
	if !SaffronGateReady(&mem) {
		return fmt.Errorf("skill: OpenSaffronGate: Fuchsia progression (#33) is incomplete")
	}

	if _, ok := guardDrinkInBag(&mem); !ok {
		if err := buySaffronGuardDrink(m, romData, policy); err != nil {
			return err
		}
	}

	// Approach from Route 7 and stop one tile before the trigger. Entering
	// (3,3) is what the ROM script observes; talking to the guard is not the
	// story trigger and would make this depend on dialogue positioning.
	stand := Destination{Map: route7GateMap, X: route7GuardStandX, Y: route7GuardStandY}
	if _, err := TravelFlee(m, romData, stand, policy, saffronGateTravelBattles); err != nil {
		return fmt.Errorf("skill: OpenSaffronGate: reach Route 7 gate: %w", err)
	}
	state.Snapshot(m, &mem)
	if SaffronGateOpen(&mem) {
		return nil
	}
	if mem.U8(sym.CurMap) != route7GateMap || mem.U8(sym.XCoord) != route7GuardStandX || mem.U8(sym.YCoord) != route7GuardStandY {
		return fmt.Errorf("skill: OpenSaffronGate: expected Route 7 guard stand (%d,%d), on map %#04x at (%d,%d)",
			route7GuardStandX, route7GuardStandY, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}

	m.Tap(emu.Right, 3, 7)
	if err := driveSaffronInteraction(m, saffronGateInteractionBudget, func(mm *state.Mem) bool {
		return SaffronGateOpen(mm) && state.Controllable(mm)
	}); err != nil {
		return fmt.Errorf("skill: OpenSaffronGate: give guard drink at trigger (%d,%d): %w",
			route7GuardTriggerX, route7GuardTriggerY, err)
	}
	state.Snapshot(m, &mem)
	if !SaffronGateOpen(&mem) {
		return fmt.Errorf("skill: OpenSaffronGate: guard interaction finished without saffron_gate_open")
	}
	return nil
}

func buySaffronGuardDrink(m *emu.Emu, romData []byte, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, ok := guardDrinkInBag(&mem); ok {
		return nil
	}
	if money := state.DecodeInventory(&mem).Money; money < freshWaterPrice {
		return fmt.Errorf("skill: OpenSaffronGate: need at least ¥%d for a guard drink, have ¥%d", freshWaterPrice, money)
	}
	if err := EnsureBagSpaceFor(m, freshWaterItem); err != nil {
		return fmt.Errorf("skill: OpenSaffronGate: make room for FRESH WATER: %w", err)
	}

	stand := Destination{Map: celadonMartRoofMap, X: vendingStandX, Y: vendingStandY}
	if _, err := TravelFlee(m, romData, stand, policy, saffronGateTravelBattles); err != nil {
		return fmt.Errorf("skill: OpenSaffronGate: reach Celadon roof vending machine: %w", err)
	}
	if err := Face(m, vendingMachineX, vendingMachineY); err != nil {
		return fmt.Errorf("skill: OpenSaffronGate: face vending machine: %w", err)
	}

	m.Tap(emu.A, 3, 7)
	if err := driveSaffronInteraction(m, saffronGateInteractionBudget, func(mm *state.Mem) bool {
		return state.DecodeInteraction(mm).Kind == state.InteractionMenu
	}); err != nil {
		return fmt.Errorf("skill: OpenSaffronGate: open vending menu: %w", err)
	}
	if err := SelectInteractionIndex(m, 0); err != nil { // FRESH WATER is the first/cheapest entry.
		return fmt.Errorf("skill: OpenSaffronGate: select FRESH WATER: %w", err)
	}
	// SelectMenuItem proves the cursor before A but intentionally returns as
	// soon as the confirm tap is sent. Give the vending handler one ordinary
	// settle window before polling for the purchase to land.
	m.StepFrames(talkSettle)
	// VendingMachineMenu (pokered/engine/events/vending_machine.asm) calls
	// HandleMenuInput exactly once, at the top; every branch after the
	// FRESH WATER/SODA POP/LEMONADE/CANCEL choice only prints text into a
	// separate box below the item list, never re-entering HandleMenuInput.
	// Nothing on screen ever redraws over that list, so its cursor glyph
	// (and MenuUp/DecodeInteraction, which key off exactly that glyph) keep
	// reading it as a live, unanswered menu for the rest of the purchase.
	// Once the one real selection above is made, that residual "menu" can
	// only ever be paged like ordinary dialogue, so tolerate it here.
	if err := driveSaffronMenuTolerant(m, saffronGateInteractionBudget, func(mm *state.Mem) bool {
		_, count := bagEntry(mm, freshWaterItem)
		return count > 0 && state.Controllable(mm)
	}); err != nil {
		return fmt.Errorf("skill: OpenSaffronGate: settle FRESH WATER purchase: %w", err)
	}
	state.Snapshot(m, &mem)
	if _, ok := guardDrinkInBag(&mem); !ok {
		return fmt.Errorf("skill: OpenSaffronGate: vending interaction returned without a valid guard drink")
	}
	return nil
}

// driveSaffronInteraction advances only ordinary dialogue while waiting for a
// concrete RAM/UI postcondition. Menus are never selected here: if an
// unexpected menu surface appears, fail closed rather than guessing.
func driveSaffronInteraction(m *emu.Emu, budget int, done func(*state.Mem) bool) error {
	return driveSaffronInteractionWithMenuPolicy(m, budget, done, false)
}

// driveSaffronMenuTolerant is driveSaffronInteraction, except a live
// InteractionMenu is paged with A instead of failing closed. It exists only
// for the wait right after a deliberate SelectInteractionIndex call, where
// the ROM has already consumed its one HandleMenuInput and any menu the
// decoder still reports is a stale cursor glyph, not a fresh input surface.
func driveSaffronMenuTolerant(m *emu.Emu, budget int, done func(*state.Mem) bool) error {
	return driveSaffronInteractionWithMenuPolicy(m, budget, done, true)
}

func driveSaffronInteractionWithMenuPolicy(m *emu.Emu, budget int, done func(*state.Mem) bool, tolerateMenu bool) error {
	var mem state.Mem
	for spent := 0; spent < budget; spent += 10 {
		state.Snapshot(m, &mem)
		if done(&mem) {
			return nil
		}
		switch interaction := state.DecodeInteraction(&mem); interaction.Kind {
		case state.InteractionDialogue:
			m.Tap(emu.A, 3, 7)
		case state.InteractionNone:
			m.StepFrames(10)
		case state.InteractionMenu:
			if tolerateMenu {
				m.Tap(emu.A, 3, 7)
				continue
			}
			// The vending-menu predicate is checked above. Any other live menu
			// means a story step reached an input surface it did not own.
			return fmt.Errorf("unexpected menu while waiting: %q", interaction.Text)
		default:
			return fmt.Errorf("unexpected interaction %q while waiting: %q", interaction.Kind, interaction.Text)
		}
	}
	state.Snapshot(m, &mem)
	interaction := state.DecodeInteraction(&mem)
	return fmt.Errorf("interaction exceeded %d frames on map %#04x at (%d,%d), surface=%q text=%q",
		budget, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), interaction.Kind, interaction.Text)
}
