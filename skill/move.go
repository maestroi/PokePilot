package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// ErrBlocked reports that a step could not be taken.
type ErrBlocked struct {
	Step world.Step
	At   struct{ X, Y uint8 }
}

func (e *ErrBlocked) Error() string {
	return fmt.Sprintf("skill: step %s blocked at (%d,%d)", e.Step, e.At.X, e.At.Y)
}

// ErrBattleInterrupted and ErrDialogueInterrupted are returned by WalkPath
// when the game leaves the overworld mid-path. Callers decide what to do;
// WalkPath never presses A to dismiss anything.
var (
	ErrBattleInterrupted   = errors.New("skill: battle interrupted movement")
	ErrDialogueInterrupted = errors.New("skill: text box interrupted movement")
)

// ponytail: the 60/40-frame budgets below are empirical, measured on this
// ROM (one tile of movement, then the step animation settling). Tighten
// them only with a measurement, not a guess.
const (
	stepMoveBudget   = 60
	stepSettleBudget = 40
)

func buttonFor(s world.Step) (emu.Button, bool) {
	switch s {
	case world.StepUp:
		return emu.Up, true
	case world.StepDown:
		return emu.Down, true
	case world.StepLeft:
		return emu.Left, true
	case world.StepRight:
		return emu.Right, true
	}
	return 0, false
}

func playerXY(m *emu.Emu) (uint8, uint8) {
	return m.Peek8(sym.XCoord), m.Peek8(sym.YCoord)
}

// StepOnce attempts a single tile of movement. It returns nil when the
// player's tile coordinate actually changed in the requested direction.
//
// The completion predicate is the coordinate change, not WalkCounter:
// WalkCounter is already 0 before a step starts, so waiting on it alone
// exits after a few frames with the player still on the same tile. A
// direction press may also only turn the player in place, so a timed-out
// attempt is retried once before the step is treated as blocked.
func StepOnce(m *emu.Emu, s world.Step) error {
	btn, ok := buttonFor(s)
	if !ok {
		return fmt.Errorf("skill: invalid step %s", s)
	}

	startX, startY := playerXY(m)
	moved := false
	for attempt := 0; attempt < 2 && !moved; attempt++ {
		_, err := m.HoldUntil(btn, stepMoveBudget, func(m *emu.Emu) bool {
			x, y := playerXY(m)
			return x != startX || y != startY
		})
		if err == nil {
			moved = true
		}
	}
	if !moved {
		return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{startX, startY}}
	}

	if _, err := m.StepUntil(stepSettleBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.WalkCounter) == 0
	}); err != nil {
		return fmt.Errorf("skill: step %s: walk animation unsettled after %d frames", s, stepSettleBudget)
	}

	x, y := playerXY(m)
	if int(x) != int(startX)+s.DX || int(y) != int(startY)+s.DY {
		return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{x, y}}
	}
	return nil
}

// WalkPath executes each step in order, re-reading state after every step.
// It stops and returns an error if a step cannot be completed, if a battle
// starts, or if a text box opens.
func WalkPath(m *emu.Emu, path []world.Step) error {
	var mem state.Mem
	for _, step := range path {
		if err := StepOnce(m, step); err != nil {
			return err
		}
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil {
			return ErrBattleInterrupted
		}
		if state.DecodeDialogue(&mem) != nil {
			return ErrDialogueInterrupted
		}
	}
	return nil
}
