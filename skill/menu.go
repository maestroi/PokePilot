package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrMenuStuck reports that the cursor would not move to the wanted index.
var ErrMenuStuck = errors.New("skill: menu cursor did not reach the target")

// menuSettleFrames bounds the wait for the cursor to move after a tap. The
// tap itself already steps 10 frames (hold 3, gap 7); this covers the menu's
// joypad poll and cursor redraw.
const menuSettleFrames = 30

// SelectMenuItem moves the cursor to index and presses A. It returns an
// error if index is out of range for the open menu, or if the cursor stops
// responding before reaching it.
//
// The selection is step-and-verify, the same shape as movement: each
// direction tap is followed by a re-read of wCurrentMenuItem, and A is
// pressed only once the cursor index is asserted to be index. Press counts
// never establish success; the cursor index is the positive fact. The loop
// moves toward the target and stops, so it never relies on wrap-around.
// wMaxMenuItem is the item count, so valid indices are 0..Max-1; an
// out-of-range index is rejected up front (chasing one would loop forever,
// since the cursor wraps and never reads as stuck). Callers gate on
// FontLoaded; SelectMenuItem does not.
func SelectMenuItem(m *emu.Emu, index int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	menu := state.DecodeMenu(&mem)
	if index < 0 || index >= menu.Max {
		return fmt.Errorf("skill: SelectMenuItem: index %d out of range for menu with max %d", index, menu.Max)
	}

	const stuckLimit = 5
	stuck := 0
	for menu.Current != index {
		btn := emu.Down
		if menu.Current > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.CurrentMenuItem)) != menu.Current
		}); err != nil {
			// The cursor did not move across the whole settle interval.
			stuck++
			if stuck >= stuckLimit {
				state.Snapshot(m, &mem)
				cur := state.DecodeMenu(&mem).Current
				return fmt.Errorf("skill: SelectMenuItem: cursor stuck at %d, wanted %d (max %d), %d consecutive taps without movement: %w",
					cur, index, menu.Max, stuck, ErrMenuStuck)
			}
		} else {
			stuck = 0
		}
		state.Snapshot(m, &mem)
		menu = state.DecodeMenu(&mem)
	}

	m.Tap(emu.A, 3, 7)
	return nil
}

// selectTwoOption selects YES (0) or NO (1) from a live TWO_OPTION_MENU.
// Unlike the Start menu consumed by SelectMenuItem, DisplayTwoOptionMenu
// stores the last valid index in wMaxMenuItem (1), not the item count. Its
// decoded prompt is the positive proof that these inclusive semantics apply.
func selectTwoOption(m *emu.Emu, index int) error {
	if index < 0 || index > 1 {
		return fmt.Errorf("skill: selectTwoOption: index %d out of range 0..1", index)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	prompt := state.DecodeTwoOptionMenu(&mem)
	if prompt == nil {
		return errors.New("skill: selectTwoOption: no two-option prompt is open")
	}

	const stuckLimit = 5
	stuck := 0
	current := prompt.Index
	for current != index {
		previous := current
		btn := emu.Down
		if previous > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.CurrentMenuItem)) != previous
		}); err != nil {
			stuck++
			if stuck >= stuckLimit {
				return fmt.Errorf("skill: selectTwoOption: cursor stuck at %d, wanted %d: %w", previous, index, ErrMenuStuck)
			}
		} else {
			stuck = 0
		}
		current = int(m.Peek8(sym.CurrentMenuItem))
		if current < 0 || current > 1 {
			return fmt.Errorf("skill: selectTwoOption: cursor moved out of range to %d", current)
		}
	}

	m.Tap(emu.A, 3, 7)
	return nil
}
