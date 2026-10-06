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

// Movement timing, measured on the real ROM: a tap of any length moves the
// player exactly one cell, and the sprite needs about 21 frames to land. Holding
// the button longer does not go further, so the controller taps and waits.
const (
	// tapFrames is how long a D-pad button is held for one cell.
	tapFrames = 2
	// moveFrames is the wait after a press before the move is checked.
	moveFrames = 24
	// tapAttempts bounds how often one step is tapped; see tapUntil. The
	// post-push input lockout outlasts three taps on 8x8-scale boards (the
	// sixth puzzle on), measured on the real ROM; six clears ten puzzles.
	tapAttempts = 6
	// settleFrames bounds the extra wait when the board is still animating:
	// the decoder reports no player while the sprite is between cells, and a
	// pushed crate is a sprite too, so the background briefly shows one crate
	// fewer than the board really has.
	settleFrames = 90
)

// walkFrameBudget is the total frame budget for a full walk to the pre-push
// square. It is a safety bound; the walk terminates early when the player
// reaches the target.
const walkFrameBudget = 64 * moveFrames

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

		state, used, ok, err := tapUntil(m, dec, btn, len(before.Crates), func(s boxxle.State) bool {
			return s.Player != nil && *s.Player == step
		})
		frames += used
		if err != nil {
			return Result{Before: before, Frames: frames, PlayerWalked: walked},
				fmt.Errorf("decode after walk step: %v", err)
		}
		if frames > walkFrameBudget {
			return Result{Before: before, Frames: frames, PlayerWalked: walked},
				fmt.Errorf("%w: walk frame budget exhausted at step %v", ErrTimeout, step)
		}
		if state.Player == nil {
			return Result{Before: state, Frames: frames, PlayerWalked: walked},
				fmt.Errorf("%w: player disappeared after walk step", ErrBlocked)
		}
		if !ok {
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
	after, used, crateMoved, err := tapUntil(m, dec, btn, len(before.Crates), func(s boxxle.State) bool {
		for _, c := range s.Crates {
			if c == push.CrateTo {
				return true
			}
		}
		return false
	})
	frames += used
	if err != nil {
		return Result{Before: before, Frames: frames, PlayerWalked: walked},
			fmt.Errorf("decode after push: %v", err)
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

// tap presses a D-pad button briefly, releases it, and steps until the move
// it started has had time to land. It returns the frames stepped.
func tap(m Machine, btn emu.Button) int {
	m.Press(btn)
	for i := 0; i < tapFrames; i++ {
		m.StepFrame()
	}
	m.Release(btn)
	for i := tapFrames; i < moveFrames; i++ {
		m.StepFrame()
	}
	return moveFrames
}

// tapUntil taps btn until done holds for the settled board, at most
// tapAttempts times. The game drops D-pad input for a short while after a push
// animation, so a tap that changed nothing is retried, and only the decoded
// board decides whether a step really happened. A step into a wall or an
// unpushable crate changes nothing on every attempt and comes back not ok.
func tapUntil(m Machine, dec Decoder, btn emu.Button, crates int, done func(boxxle.State) bool) (boxxle.State, int, bool, error) {
	var (
		state  boxxle.State
		frames int
	)
	for attempt := 0; attempt < tapAttempts; attempt++ {
		frames += tap(m, btn)
		var (
			extra int
			err   error
		)
		state, extra, err = decodeSettled(m, dec, crates)
		frames += extra
		if err != nil {
			return state, frames, false, err
		}
		if done(state) {
			return state, frames, true, nil
		}
	}
	return state, frames, false, nil
}

// decodeSettled decodes the board, waiting up to settleFrames for the move to
// finish: the player has landed and every crate is back on the background.
// Moves never add or remove crates, so crates is the count before the move.
// It returns the extra frames stepped.
func decodeSettled(m Machine, dec Decoder, crates int) (boxxle.State, int, error) {
	extra := 0
	for {
		state, err := dec.DecodeBoxxleState(m)
		if err != nil || (state.Player != nil && len(state.Crates) == crates) || extra >= settleFrames {
			return state, extra, err
		}
		m.StepFrame()
		extra++
	}
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
