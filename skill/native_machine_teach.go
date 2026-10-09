package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

const (
	nativeMachineTeachBudget = 5000

	// Leaving the pack and then the START menu is two answered screens. Either
	// answer can land inside the game's post-press input blackout and vanish,
	// so each step proves its own semantic postcondition and repeats the press
	// instead of trusting a single one.
	packExitAttempts     = 4
	packExitWindowFrames = 200
)

// returnToStartMenu answers the carried-items PACK and waits for the START menu
// to become live again, repeating the press while an earlier one was swallowed
// by the game's post-press input blackout.
func returnToStartMenu(m menuMachine, decoder game.MenuDecoder, press int) bool {
	for attempt := 0; attempt < packExitAttempts; attempt++ {
		m.Tap(emu.B, press, 7)
		if waitMenuUntil(m, packExitWindowFrames, func() bool {
			return decoder.DecodeStartMenu(m).Ready
		}) {
			return true
		}
	}
	return false
}

func nativeMachineProtectedMoves(field game.FieldMoveDecoder) map[uint16]bool {
	protected := map[uint16]bool{}
	for _, id := range game.ProgressionFieldMoves() {
		native, ok := field.NativeFieldMove(id)
		if ok && native.MoveID != 0 {
			protected[native.MoveID] = true
		}
	}
	return protected
}

// nativeMachineReplacementSlot returns -1 when an empty slot exists, -2 when
// nothing is safely replaceable, or a concrete 0..3 replacement slot. The
// generic controller conservatively preserves every move the active profile
// exposes as a semantic field move; this is slightly stricter than HM-only
// permanence but can never propose deleting a required traversal capability.
func nativeMachineReplacementSlot(field game.FieldMoveDecoder, moves [4]uint16) int {
	for _, move := range moves {
		if move == 0 {
			return -1
		}
	}
	protected := nativeMachineProtectedMoves(field)
	for slot := len(moves) - 1; slot >= 0; slot-- {
		if !protected[moves[slot]] {
			return slot
		}
	}
	return -2
}

func nativeMachineCarrier(
	field game.FieldMoveDecoder,
	capability game.FieldMoveCapability,
	move FieldMove,
	execution game.BattleExecutionState,
) (partySlot, replaceSlot int, err error) {
	if len(capability.CompatiblePartySlots) == 0 {
		return -1, -2, fmt.Errorf("no compatible party carrier")
	}
	slots := append([]int(nil), capability.CompatiblePartySlots...)
	if move == FieldCut || move == FieldFlash {
		for i, j := 0, len(slots)-1; i < j; i, j = i+1, j-1 {
			slots[i], slots[j] = slots[j], slots[i]
		}
	}

	// Prefer a carrier with an empty move slot before deleting anything.
	for _, slot := range slots {
		if slot < 0 || slot >= len(execution.PartyMoves) {
			continue
		}
		if nativeMachineReplacementSlot(field, execution.PartyMoves[slot]) == -1 {
			return slot, -1, nil
		}
	}
	for _, slot := range slots {
		if slot < 0 || slot >= len(execution.PartyMoves) {
			continue
		}
		if replace := nativeMachineReplacementSlot(field, execution.PartyMoves[slot]); replace >= 0 {
			return slot, replace, nil
		}
	}
	return -1, -2, fmt.Errorf("compatible party has no safely replaceable move slot")
}

func battleStateKnowsMove(state game.BattleExecutionState, partySlot int, move uint16) bool {
	if partySlot < 0 || partySlot >= len(state.PartyMoves) {
		return false
	}
	for _, known := range state.PartyMoves[partySlot] {
		if known == move {
			return true
		}
	}
	return false
}

func teachFieldMoveWithMachineMenu(
	m *emu.Emu,
	field game.FieldMoveDecoder,
	machines game.MachineMenuDecoder,
	move FieldMove,
	native game.NativeFieldMove,
) error {
	execution, err := battleExecutionDecoderFor(m)
	if err != nil {
		return err
	}
	capability, err := fieldMoveCapabilityWithProfile(field, m, m.ROM(), move)
	if err != nil {
		return err
	}
	partySlot, replaceSlot, err := nativeMachineCarrier(field, capability, move, execution.DecodeBattleExecution(m))
	if err != nil {
		return fmt.Errorf("skill: native machine teach: choose carrier: %w", err)
	}
	return teachNativeMachine(m, machines, native, partySlot, replaceSlot)
}

// TeachNativeMachine teaches one owned TM/HM to partySlot through a profile's
// semantic machine menu, forgetting replaceSlot when the move list is full
// (-1 when an empty slot is expected). The carrier and replacement are the
// caller's decision; success is proven by the party slot knowing the move.
func TeachNativeMachine(m *emu.Emu, native game.NativeFieldMove, partySlot, replaceSlot int) error {
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return err
	}
	machines, ok := profile.(game.MachineMenuDecoder)
	if !ok {
		return fmt.Errorf("skill: native machine teach: profile %s has no machine menu", profile.ID())
	}
	return teachNativeMachine(m, machines, native, partySlot, replaceSlot)
}

func teachNativeMachine(m *emu.Emu, machines game.MachineMenuDecoder, native game.NativeFieldMove, partySlot, replaceSlot int) error {
	menu, err := menuDecoderFor(m)
	if err != nil {
		return err
	}
	// Every press below lands on Gold/Silver's native menus, so it needs the
	// profile's press hold rather than the Gen-I default. A dropped press here
	// is invisible: the pack, the START menu and the paging loop all look the
	// same whether the game ignored the key or never sampled it.
	press := menuPressHold(menu)
	party, err := partyMenuDecoderFor(m)
	if err != nil {
		return err
	}
	execution, err := battleExecutionDecoderFor(m)
	if err != nil {
		return err
	}
	if battleStateKnowsMove(execution.DecodeBattleExecution(m), partySlot, native.MoveID) {
		return nil
	}

	if err := ensureMachineMenuOpenWithDecoder(m, menu, machines); err != nil {
		return fmt.Errorf("skill: native machine teach: open PACK: %w", err)
	}
	if err := selectMachineEntryWithDecoder(m, machines, native); err != nil {
		return fmt.Errorf("skill: native machine teach: select machine: %w", err)
	}

	// Selecting a TM/HM opens its USE/QUIT submenu with USE first.
	if err := selectMenuItemWithDecoder(m, menu, 0); err != nil {
		return fmt.Errorf("skill: native machine teach: choose USE: %w", err)
	}

	// AskTeachTMHM can show one or more text pages before YES/NO. Page only
	// while neither the prompt nor the desired party menu owns input.
	for frames := 0; frames < 1600; frames += 20 {
		liveParty := party.DecodePartyMenu(m)
		if liveParty.Visible && liveParty.Kind == game.PartyMenuTeachMachine {
			break
		}
		if _, open := menu.DecodeTwoOption(m); open {
			if err := selectTwoOptionWithDecoder(m, menu, 0); err != nil {
				return fmt.Errorf("skill: native machine teach: confirm teaching: %w", err)
			}
			continue
		}
		m.Tap(emu.A, press, 7)
		m.StepFrames(20)
	}
	liveParty := party.DecodePartyMenu(m)
	if !liveParty.Visible || liveParty.Kind != game.PartyMenuTeachMachine {
		return fmt.Errorf("skill: native machine teach: teaching party menu did not appear")
	}
	if err := selectPartySlotWithDecoder(m, party, partySlot); err != nil {
		return fmt.Errorf("skill: native machine teach: select party slot %d: %w", partySlot, err)
	}

	for frames := 0; frames < nativeMachineTeachBudget; frames += 20 {
		live := execution.DecodeBattleExecution(m)
		if battleStateKnowsMove(live, partySlot, native.MoveID) {
			// TeachTMHM returns to the pack after its success text is paged.
			for settle := 0; settle < 120; settle++ {
				pack := machines.DecodeMachineMenu(m)
				if pack.Visible && pack.Pocket == game.MachinePocketTMHM && pack.Ready {
					if !returnToStartMenu(m, menu, press) {
						return fmt.Errorf("skill: native machine teach: PACK did not return to START menu")
					}
					if err := closeStartMenuWithDecoder(m, menu); err != nil {
						return fmt.Errorf("skill: native machine teach: %w", err)
					}
					return nil
				}
				m.Tap(emu.A, press, 7)
				m.StepFrames(20)
			}
			return fmt.Errorf("skill: native machine teach: learned move but PACK did not recover")
		}

		switch live.Phase {
		case game.BattleExecutionTryLearnPrompt:
			if _, open := menu.DecodeTwoOption(m); open {
				if err := selectTwoOptionWithDecoder(m, menu, 0); err != nil {
					return fmt.Errorf("skill: native machine teach: accept replacement: %w", err)
				}
			} else {
				m.Tap(emu.A, press, 7)
			}
		case game.BattleExecutionForgetMove:
			if replaceSlot < 0 {
				return fmt.Errorf("skill: native machine teach: game requested replacement despite pre-teach empty slot")
			}
			if err := selectForgetSlot(m, replaceSlot); err != nil {
				return fmt.Errorf("skill: native machine teach: choose forget slot %d: %w", replaceSlot, err)
			}
		case game.BattleExecutionHMForgetRejected:
			return fmt.Errorf("skill: native machine teach: proposed move slot %d was rejected as permanent", replaceSlot)
		case game.BattleExecutionAbandonLearn:
			return fmt.Errorf("skill: native machine teach: move-learning flow reached abandon prompt unexpectedly")
		default:
			m.Tap(emu.A, press, 7)
		}
		m.StepFrames(20)
	}
	return fmt.Errorf("skill: native machine teach: move %#04x was not learned by party slot %d within %d frames",
		native.MoveID, partySlot, nativeMachineTeachBudget)
}
