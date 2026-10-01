package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const nativeMachineTeachBudget = 5000

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
	menu, err := menuDecoderFor(m)
	if err != nil {
		return err
	}
	party, err := partyMenuDecoderFor(m)
	if err != nil {
		return err
	}
	execution, err := battleExecutionDecoderFor(m)
	if err != nil {
		return err
	}

	capability, err := fieldMoveCapabilityWithProfile(field, m, m.ROM(), move)
	if err != nil {
		return err
	}
	before := execution.DecodeBattleExecution(m)
	partySlot, replaceSlot, err := nativeMachineCarrier(field, capability, move, before)
	if err != nil {
		return fmt.Errorf("skill: native machine teach: choose carrier: %w", err)
	}

	if err := openStartMenuEntryWithDecoder(m, menu, startMenuItems); err != nil {
		return fmt.Errorf("skill: native machine teach: open PACK: %w", err)
	}
	if !waitMenuUntil(m, machineMenuSettleBudget, func() bool {
		return machines.DecodeMachineMenu(m).Visible
	}) {
		return fmt.Errorf("skill: native machine teach: PACK did not appear")
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
		m.Tap(emu.A, 3, 7)
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
					m.Tap(emu.B, 3, 7)
					if !waitMenuUntil(m, 600, func() bool {
						return menu.DecodeStartMenu(m).Ready
					}) {
						return fmt.Errorf("skill: native machine teach: PACK did not return to START menu")
					}
					m.Tap(emu.B, 3, 7)
					if !waitMenuUntil(m, 300, func() bool {
						return !menu.DecodeStartMenu(m).Visible
					}) {
						return fmt.Errorf("skill: native machine teach: START menu did not close")
					}
					return nil
				}
				m.Tap(emu.A, 3, 7)
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
				m.Tap(emu.A, 3, 7)
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
			m.Tap(emu.A, 3, 7)
		}
		m.StepFrames(20)
	}
	return fmt.Errorf("skill: native machine teach: move %#04x was not learned by party slot %d within %d frames",
		native.MoveID, partySlot, nativeMachineTeachBudget)
}
