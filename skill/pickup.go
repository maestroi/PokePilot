package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrBagNotRisen reports that pressing A did not collect the wanted item:
// the bag count for it is not exactly one higher than before. A ball that
// was already collected fails here, cleanly — there is no event flag for
// ground items (verified: red/state decodes eight story events and none is
// a pickup), so a vanished ball has no data source to explain it, and the
// bag postcondition is the whole proof.
var ErrBagNotRisen = errors.New("skill: Pickup: bag count did not rise")

// ErrPickupMenu reports that a two-option menu appeared while paging the
// pickup text. A blind A into a yes/no prompt has cost this project a caught
// Caterpie (S6-3) and a learned move (S6-4); meeting one here is a finding
// to report, not a case to handle — no ground item in Red asks a question.
var ErrPickupMenu = errors.New("skill: Pickup: a two-option menu appeared while paging the pickup text")

// Pickup walks to the tile adjacent to an item ball, faces it and takes it.
// The postcondition is the bag: Pickup succeeds only when the count for want
// rose. A text box opening is not evidence.
func Pickup(m *emu.Emu, romData []byte, x, y uint8, want uint8) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	before := bagCount(state.DecodeInventory(&mem).Items, want)

	if err := Approach(m, romData, x, y); err != nil {
		return err
	}
	if err := Face(m, x, y); err != nil {
		return fmt.Errorf("skill: Pickup: %w", err)
	}

	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0
	}); err != nil {
		return fmt.Errorf("skill: Pickup: A at (%d,%d) opened no text box: %w", x, y, ErrNoDialogue)
	}

	// Page the box closed. Before every A, check for a two-option menu and
	// STOP if one is up: the first loop pass checks the box the first press
	// opened, so a reflex A never answers a question on this path.
	for m.Peek8(sym.FontLoaded) != 0 {
		state.Snapshot(m, &mem)
		if menu := state.DecodeTwoOptionMenu(&mem); menu != nil {
			return fmt.Errorf("%w (cursor on option %d)", ErrPickupMenu, menu.Index)
		}
		m.Tap(emu.A, 3, 7)
		m.StepFrames(talkSettle)
	}

	// Same settle as Talk: the box is down, but wJoyIgnore may clear a few
	// frames after wFontLoaded.
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		if _, err := m.StepUntil(talkSettle, func(m *emu.Emu) bool {
			state.Snapshot(m, &mem)
			return state.Controllable(&mem)
		}); err != nil {
			return fmt.Errorf("skill: Pickup: not controllable %d frames after the box closed", talkSettle)
		}
	}

	state.Snapshot(m, &mem)
	after := bagCount(state.DecodeInventory(&mem).Items, want)
	if after != before+1 {
		return fmt.Errorf("%w: item %d was %d before and %d after", ErrBagNotRisen, want, before, after)
	}
	return nil
}
