// Package control executes deterministic Boxxle (Sokoban) crate pushes from
// semantic state.
//
// The controller is the closed-loop executor: it pathfinds the player to the
// pre-push square, walks there with ordinary Game Boy D-pad input, presses the
// D-pad in the push direction to move the crate, and verifies the crate
// actually moved. Every discrete step is checked against freshly decoded
// semantic state; a rejected step is returned as a typed error, never a
// silent success.
package control

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// stepFrameBudget is the number of emulator frames to step after pressing a
// D-pad button before checking whether the player or crate moved.
const stepFrameBudget = 12

// walkFrameBudget is the total frame budget for a full walk to the pre-push
// square. It is a safety bound; the walk terminates early when the player
// reaches the target.
const walkFrameBudget = 600

// pushFrameBudget is the total frame budget for the push step itself.
const pushFrameBudget = 60

var (
	// ErrNotReady is returned when the board is not a usable puzzle.
	ErrNotReady = errors.New("boxxle control: board not ready")
	// ErrBlocked is returned when a walk or push step is blocked.
	ErrBlocked = errors.New("boxxle control: step blocked")
	// ErrTimeout is returned when the frame budget is exhausted.
	ErrTimeout = errors.New("boxxle control: frame budget exhausted")
)

// Machine is the smallest emulator surface required by the Boxxle controller.
// The semantic decoder reads memory through MemoryReader while the executor
// controls only ordinary Game Boy buttons and one frame of time at a time.
type Machine interface {
	game.MemoryReader
	Press(emu.Button)
	Release(emu.Button)
	StepFrame()
}

// Decoder decodes the current board from the emulator's memory.
type Decoder interface {
	DecodeBoxxleState(game.MemoryReader) (boxxle.State, error)
}

// Result records one push attempt.
type Result struct {
	Push         boxxle.LegalPush `json:"push"`
	Before       boxxle.State     `json:"before"`
	After        boxxle.State     `json:"after"`
	Frames       int              `json:"frames"`
	CrateMoved   bool             `json:"crate_moved"`
	PlayerWalked int              `json:"player_walked"`
}

// dirButton maps a board direction to the Game Boy D-pad button.
func dirButton(d boxxle.Direction) emu.Button {
	switch d {
	case boxxle.DirUp:
		return emu.Up
	case boxxle.DirDown:
		return emu.Down
	case boxxle.DirLeft:
		return emu.Left
	case boxxle.DirRight:
		return emu.Right
	}
	return emu.Up
}

// Push executes a legal crate push: it walks the player to the pre-push
// square, presses the D-pad in the push direction, and verifies the crate
// moved to the expected cell.
//
// The function is closed-loop: after each walk step and the push step, it
// re-decodes the board and checks the player/crate position. A blocked step
// is returned as a typed error with the live state, never a silent success.
func Push(m Machine, dec Decoder, push boxxle.LegalPush) (Result, error) {
	before, err := dec.DecodeBoxxleState(m)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrNotReady, err)
	}
	if before.Screen != boxxle.ScreenPuzzle && before.Screen != boxxle.ScreenSolved {
		return Result{Before: before}, fmt.Errorf("%w: screen=%s", ErrNotReady, before.Screen)
	}

	frames := 0

	// Walk the player to the pre-push square.
	board, err := boxxle.NewBoard(before)
	if err != nil {
		return Result{Before: before}, fmt.Errorf("%w: %v", ErrNotReady, err)
	}
	path, ok := board.PathTo(*before.Player, push.PlayerFrom)
	if !ok {
		return Result{Before: before}, fmt.Errorf("%w: player cannot reach pre-push square %v", ErrBlocked, push.PlayerFrom)
	}

	walked := 0
	cur := *before.Player
	for _, step := range path {
		dir := directionBetween(cur, step)
		btn := dirButton(dir)

		m.Press(btn)
		for i := 0; i < stepFrameBudget; i++ {
			m.StepFrame()
			frames++
		}
		m.Release(btn)

		if frames > walkFrameBudget {
			return Result{Before: before, Frames: frames, PlayerWalked: walked},
				fmt.Errorf("%w: walk frame budget exhausted at step %v", ErrTimeout, step)
		}

		// Re-decode and check the player moved.
		state, err := dec.DecodeBoxxleState(m)
		if err != nil {
			return Result{Before: before, Frames: frames, PlayerWalked: walked},
				fmt.Errorf("decode after walk step: %v", err)
		}
		if state.Player == nil {
			return Result{Before: state, Frames: frames, PlayerWalked: walked},
				fmt.Errorf("%w: player disappeared after walk step", ErrBlocked)
		}
		if *state.Player != step {
			return Result{Before: state, Frames: frames, PlayerWalked: walked},
				fmt.Errorf("%w: player at %v, expected %v after step %v", ErrBlocked, *state.Player, step, dir)
		}
		cur = *state.Player
		before = state
		walked++
	}

	// Push the crate.
	dir := push.Dir
	btn := dirButton(dir)
	m.Press(btn)
	for i := 0; i < pushFrameBudget; i++ {
		m.StepFrame()
		frames++
	}
	m.Release(btn)

	after, err := dec.DecodeBoxxleState(m)
	if err != nil {
		return Result{Before: before, Frames: frames, PlayerWalked: walked},
			fmt.Errorf("decode after push: %v", err)
	}

	// Verify the crate moved to the expected cell.
	crateMoved := false
	for _, c := range after.Crates {
		if c == push.CrateTo {
			crateMoved = true
			break
		}
	}
	if !crateMoved {
		return Result{Before: before, After: after, Frames: frames, PlayerWalked: walked},
			fmt.Errorf("%w: crate did not move to %v", ErrBlocked, push.CrateTo)
	}

	return Result{
		Push:         push,
		Before:       before,
		After:        after,
		Frames:       frames,
		CrateMoved:   true,
		PlayerWalked: walked,
	}, nil
}

// directionBetween returns the direction from a to b, assuming they are
// adjacent.
func directionBetween(a, b boxxle.Pos) boxxle.Direction {
	if b.X > a.X {
		return boxxle.DirRight
	}
	if b.X < a.X {
		return boxxle.DirLeft
	}
	if b.Y > a.Y {
		return boxxle.DirDown
	}
	return boxxle.DirUp
}
