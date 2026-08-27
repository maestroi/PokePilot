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
		stepErr := StepOnce(m, step)

		// Check for an interruption BEFORE trusting stepErr. A wild
		// encounter fires mid-step: the battle freezes the player, so
		// StepOnce times out and reports the tile as blocked. Reporting
		// that as collision sent a Route 1 investigation chasing a
		// pathfinding bug that did not exist — the tile was walkable and
		// a Pidgey was on the screen.
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil {
			return ErrBattleInterrupted
		}
		if state.DecodeDialogue(&mem) != nil {
			return ErrDialogueInterrupted
		}
		if stepErr != nil {
			return stepErr
		}
	}
	return nil
}

// ponytail: maxWalkRetries and npcWaitFrames are knobs, not laws. Six
// attempts covers a sprite that wanders across a corridor twice;
// npcWaitFrames is about one and a half NPC steps on this ROM. Tune with a
// measurement, not a guess.
const (
	maxWalkRetries = 6
	npcWaitFrames  = 48
)

// walkAround walks a planned path, re-planning around obstacles the static
// collision grid cannot know about: sprites stand in doorways and wander
// into gaps, and only walking into one discovers it.
//
// readBlocked returns the tiles live sprites occupy right now, decoded from
// a fresh RAM snapshot; it is called exactly once at the top of every
// attempt, and that snapshot is what plan receives. plan must re-read the
// player's position, because a partially-walked path leaves them somewhere
// new. walk performs the steps. wait lets game time pass.
//
// Live sprite positions are ephemeral observations, never learned world
// geometry. Every plan rebuilds blockers from current RAM. A collision
// retries from new RAM; no blocker cache exists, so there is nothing to
// expire or forget.
//
// Two known races, both absorbed by the retry and neither a reason for a
// cache:
//
//   - The liveness filter is the sprite's IMAGEINDEX, which is the screen
//     overlay's state, so the snapshot is screen-local: a sprite that just
//     walked off the edge of the screen still decodes live on its last
//     tile until the overlay catches up, and one that just entered may not
//     be in it yet.
//   - TryWalking writes a walking NPC's DESTINATION tile at the start of
//     its 16-frame animation, so a sprite mid-step can straddle two tiles:
//     the snapshot may report the tile it is leaving, not the one it is
//     entering.
func walkAround(readBlocked func() map[[2]int]bool, plan func(blocked map[[2]int]bool) ([]world.Step, error), walk func([]world.Step) error, wait func()) error {
	for attempt := 0; ; attempt++ {
		blocked := readBlocked()
		steps, err := plan(blocked)
		if err != nil {
			// A plan error with live blockers present is a sprite problem,
			// not a grid problem: the grid does not lie, so the snapshot is
			// the first thing to doubt. With no live blockers the static
			// path error is real and is returned unchanged.
			if len(blocked) == 0 || attempt >= maxWalkRetries {
				return err
			}
			wait()
			continue
		}
		if err := walk(steps); err != nil {
			var eb *ErrBlocked
			if !errors.As(err, &eb) || attempt >= maxWalkRetries {
				return err
			}
			wait() // give a wandering sprite time to move on; the next
			// attempt reads a fresh snapshot
			continue
		}
		return nil
	}
}
