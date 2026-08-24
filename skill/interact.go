package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// ErrNoDialogue reports that pressing A did not open a text box.
var ErrNoDialogue = errors.New("skill: A did not open a text box")

// ponytail: the budgets below are empirical, measured on this ROM. The
// A-press cadence in Talk matters: the same TV sign took 10 presses at a
// 40-frame cadence and 6 at a 100-frame cadence, so each press is followed
// by a settle interval that keeps the cadence in the cheaper regime.
const (
	faceTurnBudget = 60  // frames for a direction tap to register as a turn
	talkOpenBudget = 120 // frames for a text box to open after pressing A
	talkSettle     = 40  // frames stepped after each A press while the box is up
	talkPressCap   = 30  // A presses before Talk gives up on a stubborn box
)

// directionTo maps a tile orthogonally adjacent to (sx,sy) to the step
// toward it. ok is false when (tx,ty) is not exactly one tile away on one
// axis.
func directionTo(sx, sy, tx, ty uint8) (world.Step, bool) {
	switch {
	case int(tx) == int(sx) && int(ty) == int(sy)-1:
		return world.StepUp, true
	case int(tx) == int(sx) && int(ty) == int(sy)+1:
		return world.StepDown, true
	case int(tx) == int(sx)-1 && int(ty) == int(sy):
		return world.StepLeft, true
	case int(tx) == int(sx)+1 && int(ty) == int(sy):
		return world.StepRight, true
	}
	return world.Step{}, false
}

func facingFor(s world.Step) state.Facing {
	switch s {
	case world.StepUp:
		return state.FacingUp
	case world.StepDown:
		return state.FacingDown
	case world.StepLeft:
		return state.FacingLeft
	case world.StepRight:
		return state.FacingRight
	}
	return 0
}

// Face turns the player to look at the orthogonally adjacent tile (tx,ty).
// It returns an error if the tile is not orthogonally adjacent, or if the
// facing did not change within the budget.
//
// The completion predicate is the decoded facing, not the position: a tap
// toward an open tile may move the player onto it, and that is fine — the
// facing is what Face promises.
func Face(m *emu.Emu, tx, ty uint8) error {
	x, y := playerXY(m)
	step, ok := directionTo(x, y, tx, ty)
	if !ok {
		return fmt.Errorf("skill: Face: tile (%d,%d) is not orthogonally adjacent to (%d,%d)", tx, ty, x, y)
	}
	btn, ok := buttonFor(step)
	if !ok {
		return fmt.Errorf("skill: Face: invalid step %s", step)
	}
	want := facingFor(step)

	m.Tap(btn, 3, 7)
	var mem state.Mem
	if _, err := m.StepUntil(faceTurnBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return state.DecodePlayer(&mem).Facing == want
	}); err != nil {
		return fmt.Errorf("skill: Face: not facing %s within %d frames", want, faceTurnBudget)
	}
	return nil
}

// Talk presses A to open a text box, then keeps pressing A while a box is
// up until it closes. It returns the number of A presses spent.
//
// The open/closed signal is FontLoaded, never TextBoxID (which read 0x01
// before, during and after the measured dialogue). The press count is
// timing-dependent, so Talk is a bounded poll and callers must not assert
// a specific count.
func Talk(m *emu.Emu) (int, error) {
	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0
	}); err != nil {
		return 0, ErrNoDialogue
	}

	presses := 1
	for m.Peek8(sym.FontLoaded) != 0 {
		if presses >= talkPressCap {
			return presses, fmt.Errorf("skill: Talk: text box still open after %d A presses", talkPressCap)
		}
		m.Tap(emu.A, 3, 7)
		presses++
		m.StepFrames(talkSettle)
	}

	// The box is down, but the game may still be settling: wJoyIgnore can
	// clear a few frames after wFontLoaded. Wait for controllable rather
	// than asserting on the very next frame.
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		if _, err := m.StepUntil(talkSettle, func(m *emu.Emu) bool {
			state.Snapshot(m, &mem)
			return state.Controllable(&mem)
		}); err != nil {
			return presses, fmt.Errorf("skill: Talk: not controllable %d frames after the box closed", talkSettle)
		}
	}
	return presses, nil
}
