package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	gen1BoxCount      = 12
	pcBoxSwitchBudget = 10000
)

// ErrPCAllBoxesFull reports that Bill's PC cannot make storage room because
// every one of Red's twelve boxes is already at the 20-Pokemon capacity.
// This is a stable collection blockage, not a menu/controller failure.
var ErrPCAllBoxesFull = errors.New("skill: Bill's PC: all boxes are full")

// pcChangeBoxSavePrompt identifies ChangeBox's mandatory save warning. A
// generic two-option menu is not enough evidence: nickname, toss, and story
// prompts use the same menu shape, so require the ROM's visible SAVE/BOX text.
func pcChangeBoxSavePrompt(mem *state.Mem) bool {
	if state.DecodeTwoOptionMenu(mem) == nil {
		return false
	}
	text := strings.ToUpper(state.ScreenText(mem))
	return strings.Contains(text, "SAVE") && strings.Contains(text, "BOX")
}

// pcChangeBoxMenuScreen identifies DisplayChangeBoxMenu from geometry alone.
// The cursor glyph can lag the menu data by a render window; recognizing the
// screen before state.MenuUp becomes true avoids sending input during render.
func pcChangeBoxMenuScreen(mem *state.Mem) bool {
	return mem.U8(sym.TopMenuItemX) == 12 &&
		mem.U8(sym.TopMenuItemY) == 1 &&
		mem.U8(sym.MaxMenuItem) == gen1BoxCount-1
}

func pcChangeBoxMenuUp(mem *state.Mem) bool {
	return state.MenuUp(mem) && pcChangeBoxMenuScreen(mem)
}

func pcBoxCounts(mem *state.Mem) [gen1BoxCount]uint8 {
	var counts [gen1BoxCount]uint8
	for i := range counts {
		counts[i] = mem.U8(sym.BoxMonCounts + uint16(i))
	}
	return counts
}

// nextNonFullBox chooses deterministically in circular order after current.
// The current box is deliberately skipped: this helper is only for recovery
// from a full active box, and returning it would make no progress.
func nextNonFullBox(counts [gen1BoxCount]uint8, current int) (int, bool) {
	if current < 0 || current >= gen1BoxCount {
		return -1, false
	}
	for step := 1; step < gen1BoxCount; step++ {
		i := (current + step) % gen1BoxCount
		if counts[i] < gen1BoxCapacity {
			return i, true
		}
	}
	return -1, false
}

// selectPCBox drives DisplayChangeBoxMenu's inclusive cursor semantics. Unlike
// the ordinary menus handled by SelectMenuItem, ChangeBox stores 11 in
// wMaxMenuItem to mean "last valid index 11", not "11 items". A dedicated
// step-and-verify helper is therefore required so Box 12 remains selectable.
func selectPCBox(m *emu.Emu, index int) error {
	if index < 0 || index >= gen1BoxCount {
		return fmt.Errorf("skill: Bill's PC: box index %d out of range 0..%d", index, gen1BoxCount-1)
	}
	if _, err := m.StepUntil(pcTransitionBudget, func(m *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return pcChangeBoxMenuUp(&mem)
	}); err != nil {
		return fmt.Errorf("skill: Bill's PC: Change Box menu cursor did not become interactive: %w", err)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	const stuckLimit = 5
	stuck := 0
	current := int(mem.U8(sym.CurrentMenuItem))
	for current != index {
		previous := current
		btn := emu.Down
		if current > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.CurrentMenuItem)) != previous
		}); err != nil {
			stuck++
			if stuck >= stuckLimit {
				return fmt.Errorf("skill: Bill's PC: Change Box cursor stuck at %d, wanted %d: %w", previous, index, ErrMenuStuck)
			}
		} else {
			stuck = 0
		}
		current = int(m.Peek8(sym.CurrentMenuItem))
	}
	m.Tap(emu.A, 3, 7)
	return nil
}

// SwitchToNextNonFullBox changes Bill's active PC box through the real PC UI.
// It never writes box data or wCurrentBoxNum directly. Selection is based on
// the twelve counts the ROM itself builds for DisplayChangeBoxMenu, and success
// is verified from the newly loaded active-box number and count after the save.
func SwitchToNextNonFullBox(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: Bill's PC: nil move policy")
	}

	var before state.Mem
	state.Snapshot(m, &before)
	active := state.DecodeBox(&before)
	if active.Count < gen1BoxCapacity {
		return nil
	}
	current := int(active.Number)
	if current >= gen1BoxCount {
		return fmt.Errorf("skill: Bill's PC: active box number %d is outside 0..%d", current, gen1BoxCount-1)
	}

	if err := openBillsPC(m, romData, policy); err != nil {
		return err
	}
	cleanup := func(err error) error {
		_ = closePCToOverworld(m)
		return err
	}

	if err := SelectMenuItem(m, 3); err != nil { // CHANGE BOX
		return cleanup(fmt.Errorf("skill: Bill's PC: select CHANGE BOX: %w", err))
	}
	if err := pcAdvanceUntil(m, billsPCMenuUp, pcChangeBoxSavePrompt, "Change Box save prompt"); err != nil {
		return cleanup(err)
	}
	if err := selectTwoOption(m, 0); err != nil { // YES
		return cleanup(fmt.Errorf("skill: Bill's PC: confirm Change Box save: %w", err))
	}

	// After YES the ROM proceeds directly into DisplayChangeBoxMenu; there is
	// no text to page. Wait passively for the screen first so a queued A cannot
	// select the current box while the new menu is still rendering.
	var menu state.Mem
	if _, err := m.StepUntil(pcTransitionBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &menu)
		return pcChangeBoxMenuScreen(&menu)
	}); err != nil {
		return cleanup(fmt.Errorf("skill: Bill's PC: Change Box menu did not appear: %w", err))
	}
	// DisplayChangeBoxMenu writes its geometry before GetMonCountsForAllBoxes.
	// The live cursor proves the routine finished, so only then is the 12-byte
	// count scratch buffer authoritative.
	if _, err := m.StepUntil(pcTransitionBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &menu)
		return pcChangeBoxMenuUp(&menu)
	}); err != nil {
		return cleanup(fmt.Errorf("skill: Bill's PC: Change Box menu did not become interactive: %w", err))
	}
	state.Snapshot(m, &menu)
	counts := pcBoxCounts(&menu)
	target, ok := nextNonFullBox(counts, current)
	if !ok {
		return cleanup(fmt.Errorf("%w: each of %d boxes has %d Pokemon", ErrPCAllBoxesFull, gen1BoxCount, gen1BoxCapacity))
	}
	if err := selectPCBox(m, target); err != nil {
		return cleanup(fmt.Errorf("skill: Bill's PC: select box %d: %w", target+1, err))
	}

	// ChangeBox saves the old box to SRAM, loads the selected one into WRAM,
	// saves game data, waits for the save sound, then returns to BillsPCMenu.
	// That can exceed the ordinary nested-menu transition budget, so this
	// positive wait gets its own larger bound.
	var after state.Mem
	for spent := 0; spent < pcBoxSwitchBudget; spent += talkSettle {
		state.Snapshot(m, &after)
		box := state.DecodeBox(&after)
		if billsPCMenuScreen(&after) && box.Number == uint8(target) {
			if box.Count >= gen1BoxCapacity {
				return cleanup(fmt.Errorf("skill: Bill's PC: changed to box %d but it is full (%d Pokemon)", target+1, box.Count))
			}
			if err := closePCToOverworld(m); err != nil {
				return err
			}
			return nil
		}
		m.StepFrames(talkSettle)
	}
	state.Snapshot(m, &after)
	box := state.DecodeBox(&after)
	return cleanup(fmt.Errorf("skill: Bill's PC: box %d did not become active within %d frames (active=%d count=%d screen=%q)",
		target+1, pcBoxSwitchBudget, box.Number+1, box.Count, state.ScreenText(&after)))
}

// EnsurePartySlotForCollection is the long-running collection variant of
// EnsurePartySlot. Ordinary story/roster callers keep the historical behavior;
// Dex catches additionally roll over a full active box before depositing a
// surplus party member.
func EnsurePartySlotForCollection(m *emu.Emu, romData []byte, policy MovePolicy, incoming uint8) error {
	if policy == nil {
		return fmt.Errorf("skill: Bill's PC: nil move policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	slot, err := planPartySlot(romData, state.DecodeParty(&mem), state.DecodeBox(&mem), state.Mon{Species: incoming}, OwnedCoreProgressionFieldMoves(romData, &mem))
	if err != nil && errors.Is(err, ErrPCBoxFull) {
		if err := SwitchToNextNonFullBox(m, romData, policy); err != nil {
			return err
		}
		state.Snapshot(m, &mem)
		slot, err = planPartySlot(romData, state.DecodeParty(&mem), state.DecodeBox(&mem), state.Mon{Species: incoming}, OwnedCoreProgressionFieldMoves(romData, &mem))
	}
	if err != nil {
		return err
	}
	if slot < 0 {
		return nil
	}
	return DepositPartyMon(m, romData, policy, slot)
}
