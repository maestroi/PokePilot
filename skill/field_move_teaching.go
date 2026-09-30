package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const numberedMachineTeachBudget = 5000

type screenTextDecoder interface {
	ScreenText(game.MemoryReader) string
}

type numberedMachineTeachingProfile interface {
	game.FieldMoveDecoder
	game.FieldMoveTeachingDecoder
	game.MenuDecoder
	game.PartyMenuDecoder
	game.BattleExecutionDecoder
	game.OverworldDecoder
	screenTextDecoder
}

func teachNumberedMachine(
	m *emu.Emu,
	profile numberedMachineTeachingProfile,
	move FieldMove,
	native game.NativeFieldMove,
) error {
	if m == nil || profile == nil {
		return fmt.Errorf("skill: numbered machine teaching: missing emulator/profile")
	}
	if native.MachineNumber == 0 {
		return fmt.Errorf("skill: numbered machine teaching: machine number is missing")
	}

	capability, err := fieldMoveCapabilityWithProfile(profile, m, m.ROM(), move)
	if err != nil {
		return err
	}
	partySlot, replaceSlot, err := numberedMachineRecipient(profile, capability, profile.DecodeBattleExecution(m))
	if err != nil {
		return err
	}

	if err := openStartMenuEntryWithDecoder(m, profile, startMenuItems); err != nil {
		return fmt.Errorf("open PACK: %w", err)
	}
	if err := openNumberedMachinePocket(m, profile); err != nil {
		return err
	}
	if err := selectNumberedMachine(m, profile, native.MachineNumber); err != nil {
		return err
	}

	// TMHMPocket returns to the Pack's USE/QUIT submenu.
	m.Tap(emu.A, 3, 7)
	if !waitMenuUntil(m, 600, func() bool {
		text := profile.ScreenText(m)
		cursor := profile.DecodeMenuCursor(m)
		return strings.Contains(text, "USE") && strings.Contains(text, "QUIT") && cursor.Max == 1
	}) {
		return fmt.Errorf("skill: numbered machine teaching: USE/QUIT menu did not appear")
	}
	if err := selectMenuItemWithDecoder(m, profile, 0); err != nil {
		return fmt.Errorf("skill: numbered machine teaching: select USE: %w", err)
	}

	// HM/TM boot text is paged before the cartridge presents the Teach? prompt.
	if !waitForMachineTeachPrompt(m, profile) {
		return fmt.Errorf("skill: numbered machine teaching: Teach prompt did not appear: %q", profile.ScreenText(m))
	}
	if err := selectTwoOptionWithDecoder(m, profile, 0); err != nil {
		return fmt.Errorf("skill: numbered machine teaching: answer Teach prompt: %w", err)
	}

	if !waitMenuUntil(m, 1000, func() bool {
		s := profile.DecodePartyMenu(m)
		return s.Visible && s.Kind == game.PartyMenuMachineTeach
	}) {
		return fmt.Errorf("skill: numbered machine teaching: target party menu did not appear: %q", profile.ScreenText(m))
	}
	if err := selectPartySlotWithDecoder(m, profile, partySlot); err != nil {
		return fmt.Errorf("skill: numbered machine teaching: select party slot %d: %w", partySlot, err)
	}

	if err := finishNumberedMachineLearning(m, profile, partySlot, replaceSlot, native.MoveID); err != nil {
		return err
	}
	if err := closeNumberedMachineUI(m, profile); err != nil {
		return err
	}
	return nil
}

func openNumberedMachinePocket(m menuMachine, profile numberedMachineTeachingProfile) error {
	if !waitMenuUntil(m, 1000, func() bool {
		return strings.Contains(profile.ScreenText(m), "POCKET")
	}) {
		return fmt.Errorf("skill: numbered machine teaching: PACK pocket did not appear")
	}
	for pocket := 0; pocket < 5; pocket++ {
		if profile.DecodeMachinePocket(m).Visible {
			return nil
		}
		m.Tap(emu.Right, 3, 7)
		if !waitMenuUntil(m, 160, func() bool {
			return strings.Contains(profile.ScreenText(m), "POCKET")
		}) {
			return fmt.Errorf("skill: numbered machine teaching: pocket switch did not settle")
		}
	}
	return fmt.Errorf("skill: numbered machine teaching: TM pocket was not reachable")
}

func selectNumberedMachine(m menuMachine, profile numberedMachineTeachingProfile, target uint16) error {
	if target == 0 {
		return fmt.Errorf("skill: numbered machine teaching: invalid machine 0")
	}
	const maxMoves = 80
	for i := 0; i < maxMoves; i++ {
		state := profile.DecodeMachinePocket(m)
		if !state.Visible {
			return fmt.Errorf("skill: numbered machine teaching: TM pocket disappeared")
		}
		if state.MachineNumber == target {
			return nil
		}
		button := emu.Down
		switch {
		case state.MachineNumber == 0:
			button = emu.Up
		case state.MachineNumber > target:
			button = emu.Up
		}
		before := state.MachineNumber
		m.Tap(button, 3, 7)
		waitMenuUntil(m, 80, func() bool {
			next := profile.DecodeMachinePocket(m)
			return next.Visible && next.MachineNumber != before
		})
	}
	state := profile.DecodeMachinePocket(m)
	return fmt.Errorf("skill: numbered machine teaching: machine %d not selected after %d cursor moves (current=%d)",
		target, maxMoves, state.MachineNumber)
}

func waitForMachineTeachPrompt(m menuMachine, profile numberedMachineTeachingProfile) bool {
	for i := 0; i < 80; i++ {
		if _, ok := profile.DecodeTwoOption(m); ok && strings.Contains(profile.ScreenText(m), "Teach") {
			return true
		}
		m.Tap(emu.A, 3, 7)
		m.StepFrames(20)
	}
	return false
}

func numberedMachineRecipient(
	profile game.FieldMoveDecoder,
	capability game.FieldMoveCapability,
	execution game.BattleExecutionState,
) (partySlot int, replaceSlot int, err error) {
	if len(capability.CompatiblePartySlots) == 0 {
		return -1, -1, fmt.Errorf("skill: numbered machine teaching: no compatible party carrier")
	}
	ordered := make([]int, 0, len(capability.CompatiblePartySlots))
	for _, slot := range capability.CompatiblePartySlots {
		if slot > 0 {
			ordered = append(ordered, slot)
		}
	}
	for _, slot := range capability.CompatiblePartySlots {
		if slot == 0 {
			ordered = append(ordered, slot)
		}
	}

	// Prefer a bench carrier with an empty slot, then the lead with an empty
	// slot. Only replace an existing move when every compatible carrier is full.
	for _, slot := range ordered {
		if slot < 0 || slot >= len(execution.PartyMoves) {
			continue
		}
		for moveSlot, known := range execution.PartyMoves[slot] {
			if known == 0 {
				return slot, moveSlot, nil
			}
		}
	}
	for _, slot := range ordered {
		if slot < 0 || slot >= len(execution.PartyMoves) {
			continue
		}
		for moveSlot, known := range execution.PartyMoves[slot] {
			if !nativeMoveIsHM(profile, known) {
				return slot, moveSlot, nil
			}
		}
	}
	return -1, -1, fmt.Errorf("skill: numbered machine teaching: compatible party has no replaceable move")
}

func nativeMoveIsHM(profile game.FieldMoveDecoder, move uint16) bool {
	if move == 0 {
		return false
	}
	for _, field := range SemanticFieldMoves() {
		id, ok := semanticFieldMove(field)
		if !ok {
			continue
		}
		native, ok := profile.NativeFieldMove(id)
		if !ok || native.MachineNumber < 51 || native.MachineNumber > 57 {
			continue
		}
		if native.MoveID == move {
			return true
		}
	}
	return false
}

func finishNumberedMachineLearning(
	m menuMachine,
	profile numberedMachineTeachingProfile,
	partySlot int,
	replaceSlot int,
	move uint16,
) error {
	for spent := 0; spent < numberedMachineTeachBudget; spent += 20 {
		execution := profile.DecodeBattleExecution(m)
		if partyKnowsNativeMove(execution, partySlot, move) {
			return nil
		}
		text := profile.ScreenText(m)
		if strings.Contains(text, "not compatible") {
			return fmt.Errorf("skill: numbered machine teaching: cartridge rejected compatible party slot %d: %q", partySlot, text)
		}
		switch execution.Phase {
		case game.BattleExecutionHMForgetRejected:
			return fmt.Errorf("skill: numbered machine teaching: replacement slot %d is an HM", replaceSlot)
		case game.BattleExecutionForgetMove:
			if replaceSlot < 0 {
				return fmt.Errorf("skill: numbered machine teaching: cartridge requested replacement despite an empty slot")
			}
			if err := selectMenuItemWithDecoder(m, profile, replaceSlot); err != nil {
				return fmt.Errorf("skill: numbered machine teaching: choose move slot %d: %w", replaceSlot, err)
			}
		case game.BattleExecutionTryLearnPrompt:
			if _, ok := profile.DecodeTwoOption(m); ok {
				if err := selectTwoOptionWithDecoder(m, profile, 0); err != nil {
					return fmt.Errorf("skill: numbered machine teaching: answer replace prompt: %w", err)
				}
			} else {
				m.Tap(emu.A, 3, 7)
			}
		case game.BattleExecutionAbandonLearn:
			return fmt.Errorf("skill: numbered machine teaching: cartridge reached abandon-learning prompt unexpectedly")
		default:
			m.Tap(emu.A, 3, 7)
		}
		m.StepFrames(20)
	}
	return fmt.Errorf("skill: numbered machine teaching: move %#04x was not learned within %d frames",
		move, numberedMachineTeachBudget)
}

func partyKnowsNativeMove(execution game.BattleExecutionState, partySlot int, move uint16) bool {
	if partySlot < 0 || partySlot >= len(execution.PartyMoves) {
		return false
	}
	for _, known := range execution.PartyMoves[partySlot] {
		if known == move {
			return true
		}
	}
	return false
}

func closeNumberedMachineUI(m menuMachine, profile numberedMachineTeachingProfile) error {
	for i := 0; i < 80; i++ {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if _, choice := profile.DecodeTwoOption(m); choice {
			return fmt.Errorf("skill: numbered machine teaching: unexpected choice prompt while closing UI")
		}
		m.Tap(emu.B, 3, 7)
		m.StepFrames(20)
	}
	return fmt.Errorf("skill: numbered machine teaching: UI did not close to the overworld: %q", profile.ScreenText(m))
}
