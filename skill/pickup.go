package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// approachViaTravel walks to a walkable tile orthogonally adjacent to
// (targetX, targetY) on the current map, resolving the wild battles that
// interrupt the way. It is a no-op when the player is already adjacent.
func approachViaTravel(m *emu.Emu, romData []byte, targetX, targetY uint8, policy MovePolicy) error {
	sx, sy := playerXY(m)
	if _, ok := directionTo(sx, sy, targetX, targetY); ok {
		return nil
	}
	px, py := int(sx), int(sy)
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return fmt.Errorf("skill: Pickup: parse map %#04x: %w", cur, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return fmt.Errorf("skill: Pickup: build map %#04x: %w", cur, err)
	}

	var best *struct{ x, y, d int }
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		nx, ny := int(targetX)+s.DX, int(targetY)+s.DY
		if !grid.InBounds(nx, ny) || !grid.Walkable(nx, ny) {
			continue
		}
		c := struct{ x, y, d int }{nx, ny, absInt(nx-px) + absInt(ny-py)}
		if best == nil || c.d < best.d {
			best = &c
		}
	}
	if best == nil {
		return fmt.Errorf("skill: Pickup: no walkable tile beside (%d,%d) on map %#04x", targetX, targetY, cur)
	}
	_, err = Travel(m, romData, Destination{Map: cur, X: uint8(best.x), Y: uint8(best.y)}, policy, 20)
	if err != nil {
		return fmt.Errorf("skill: Pickup: approach beside (%d,%d) on map %#04x: %w", targetX, targetY, cur, err)
	}
	return nil
}

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
//
// The approach uses Travel, not Approach: ground items sit in tall grass
// (the forest's antidote), and Approach aborts on the first wild battle by
// design — a pickup objective there would fail on every retry. Travel fights
// through the encounters; a blackout ends the approach as ErrBlackedOut,
// which is a recoverable outcome for the caller, not a dead end.
func Pickup(m *emu.Emu, romData []byte, x, y uint8, want uint8, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	before := bagCount(state.DecodeInventory(&mem).Items, want)

	if err := approachViaTravel(m, romData, x, y, policy); err != nil {
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
