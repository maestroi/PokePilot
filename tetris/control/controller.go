// Package control executes deterministic Tetris inputs from semantic state.
package control

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/tetris"
)

const (
	dropFrameBudget   = 180
	settleFrameBudget = 480
)

var (
	ErrNotReady = errors.New("tetris control: piece input not ready")
	ErrBlocked  = errors.New("tetris control: requested placement is blocked")
	ErrTimeout  = errors.New("tetris control: placement timed out")
)

// Machine is the smallest emulator surface required by the Tetris controller.
// The semantic decoder reads memory through MemoryReader while the executor
// controls only ordinary Game Boy buttons and one frame of time at a time.
type Machine interface {
	game.MemoryReader
	Press(emu.Button)
	Release(emu.Button)
	StepFrame()
}

// Placement addresses the same anchor coordinates exposed by
// tetris.PieceState. Column is the active piece anchor column, not necessarily
// the left-most occupied cell of the tetromino.
type Placement struct {
	Rotation uint8 `json:"rotation"`
	Column   int   `json:"column"`
}

// Result records the verified semantic effect of one placement.
type Result struct {
	Piece        tetris.Piece `json:"piece"`
	Target       Placement    `json:"target"`
	Before       tetris.State `json:"before"`
	After        tetris.State `json:"after"`
	Frames       int          `json:"frames"`
	LinesCleared int          `json:"lines_cleared"`
	BoardChanged bool         `json:"board_changed"`
}

// Place rotates and shifts the current tetromino to target, soft-drops it, and
// waits until the lock/line-clear transition has either produced the next
// controllable piece or reached a terminal state.
//
// The function is intentionally closed-loop: every discrete rotation/shift is
// checked against freshly decoded semantic state. A rejected input is returned
// as ErrBlocked rather than silently continuing with a different placement.
func Place(profile game.CartridgeProfile, m Machine, target Placement) (Result, error) {
	if profile == nil {
		return Result{}, fmt.Errorf("%w: nil cartridge profile", ErrNotReady)
	}
	if m == nil {
		return Result{}, fmt.Errorf("%w: nil machine", ErrNotReady)
	}
	if target.Rotation > 3 {
		return Result{}, fmt.Errorf("%w: rotation %d is outside 0..3", ErrBlocked, target.Rotation)
	}
	if target.Column < 0 || target.Column >= tetris.BoardWidth {
		return Result{}, fmt.Errorf("%w: column %d is outside 0..%d", ErrBlocked, target.Column, tetris.BoardWidth-1)
	}

	before, err := tetris.Observe(profile, m)
	if err != nil {
		return Result{}, err
	}
	if before.Screen != tetris.ScreenPlaying {
		return Result{}, fmt.Errorf("%w: screen is %s", ErrNotReady, before.Screen)
	}
	if before.Paused {
		return Result{}, fmt.Errorf("%w: game is paused", ErrNotReady)
	}
	if !before.ReadyForPieceInput || before.Active == nil {
		return Result{}, fmt.Errorf("%w: lock/clear transition is active", ErrNotReady)
	}

	result := Result{
		Piece:  before.Active.Piece,
		Target: target,
		Before: before,
	}

	current := before
	steps, err := rotateTo(profile, m, current, target.Rotation)
	result.Frames += steps
	if err != nil {
		return result, err
	}
	current, err = tetris.Observe(profile, m)
	if err != nil {
		return result, err
	}

	steps, err = shiftTo(profile, m, current, target.Column)
	result.Frames += steps
	if err != nil {
		return result, err
	}
	current, err = tetris.Observe(profile, m)
	if err != nil {
		return result, err
	}
	if current.Active == nil ||
		current.Active.Piece != before.Active.Piece ||
		current.Active.Rotation != target.Rotation ||
		current.Active.X != target.Column {
		return result, fmt.Errorf("%w: target drifted before drop", ErrBlocked)
	}

	after, frames, err := softDropAndSettle(profile, m)
	result.Frames += frames
	result.After = after
	result.BoardChanged = after.Board != before.Board
	result.LinesCleared = after.LinesCleared - before.LinesCleared
	if result.LinesCleared < 0 {
		result.LinesCleared = 0
	}
	if err != nil {
		return result, err
	}

	if !placementEffectObserved(before, after) {
		return result, fmt.Errorf("%w: lock transition completed without a board, line, or terminal-state change", ErrBlocked)
	}
	return result, nil
}

func rotateTo(profile game.CartridgeProfile, m Machine, state tetris.State, target uint8) (int, error) {
	if state.Active == nil {
		return 0, fmt.Errorf("%w: active piece disappeared before rotation", ErrNotReady)
	}
	current := state.Active.Rotation
	if current == target {
		return 0, nil
	}

	clockwise := int((current - target + 4) % 4)        // A decrements raw orientation.
	counterClockwise := int((target - current + 4) % 4) // B increments it.
	button := emu.A
	steps := clockwise
	delta := uint8(3) // -1 mod 4
	if counterClockwise < clockwise {
		button = emu.B
		steps = counterClockwise
		delta = 1
	}

	frames := 0
	expected := current
	for i := 0; i < steps; i++ {
		expected = (expected + delta) & 3
		frames += pulse(m, button)

		next, err := tetris.Observe(profile, m)
		if err != nil {
			return frames, err
		}
		if err := requireSamePieceReady(state.Active.Piece, next); err != nil {
			return frames, err
		}
		if next.Active.Rotation != expected {
			return frames, fmt.Errorf(
				"%w: rotation input rejected for %s (wanted %d, still %d)",
				ErrBlocked, state.Active.Piece, expected, next.Active.Rotation,
			)
		}
	}
	return frames, nil
}

func shiftTo(profile game.CartridgeProfile, m Machine, state tetris.State, target int) (int, error) {
	if state.Active == nil {
		return 0, fmt.Errorf("%w: active piece disappeared before shift", ErrNotReady)
	}
	button := emu.Right
	delta := 1
	if target < state.Active.X {
		button = emu.Left
		delta = -1
	}

	frames := 0
	current := state.Active.X
	for current != target {
		expected := current + delta
		frames += pulse(m, button)

		next, err := tetris.Observe(profile, m)
		if err != nil {
			return frames, err
		}
		if err := requireSamePieceReady(state.Active.Piece, next); err != nil {
			return frames, err
		}
		if next.Active.X != expected {
			return frames, fmt.Errorf(
				"%w: horizontal input rejected for %s (wanted column %d, still %d)",
				ErrBlocked, state.Active.Piece, expected, next.Active.X,
			)
		}
		current = next.Active.X
	}
	return frames, nil
}

func softDropAndSettle(profile game.CartridgeProfile, m Machine) (tetris.State, int, error) {
	m.Press(emu.Down)
	downHeld := true
	releaseDown := func() {
		if downHeld {
			m.Release(emu.Down)
			downHeld = false
		}
	}
	defer releaseDown()

	frames := 0
	var state tetris.State
	sawTransition := false
	for i := 0; i < dropFrameBudget; i++ {
		m.StepFrame()
		frames++

		var err error
		state, err = tetris.Observe(profile, m)
		if err != nil {
			return state, frames, err
		}
		if state.GameOver || state.Complete {
			sawTransition = true
			break
		}
		if state.Locking || state.Clearing {
			sawTransition = true
			break
		}
	}
	if !sawTransition {
		return state, frames, fmt.Errorf("%w: piece did not begin locking within %d frames", ErrTimeout, dropFrameBudget)
	}

	releaseDown()
	// Give the joypad one neutral frame so a later placement can generate a
	// fresh Down edge even if the next piece becomes ready immediately.
	m.StepFrame()
	frames++

	state, err := tetris.Observe(profile, m)
	if err != nil {
		return state, frames, err
	}
	if state.GameOver || state.Complete {
		return state, frames, nil
	}
	if state.ReadyForPieceInput {
		return state, frames, nil
	}

	for i := 0; i < settleFrameBudget; i++ {
		m.StepFrame()
		frames++

		state, err = tetris.Observe(profile, m)
		if err != nil {
			return state, frames, err
		}
		if state.GameOver || state.Complete || state.ReadyForPieceInput {
			return state, frames, nil
		}
	}
	return state, frames, fmt.Errorf("%w: lock/clear transition did not settle within %d frames", ErrTimeout, settleFrameBudget)
}

func requireSamePieceReady(piece tetris.Piece, state tetris.State) error {
	if state.GameOver || state.Complete {
		return fmt.Errorf("%w: game became terminal while positioning %s", ErrBlocked, piece)
	}
	if !state.ReadyForPieceInput || state.Active == nil {
		return fmt.Errorf("%w: %s began locking before positioning completed", ErrBlocked, piece)
	}
	if state.Active.Piece != piece {
		return fmt.Errorf("%w: active piece changed from %s to %s during positioning", ErrBlocked, piece, state.Active.Piece)
	}
	return nil
}

// pulse gives hJoyPressed one pressed frame and one released frame. Tetris
// movement is edge-driven for rotations and initial horizontal shifts, so this
// avoids relying on DAS/key-repeat timing.
func pulse(m Machine, button emu.Button) int {
	m.Press(button)
	m.StepFrame()
	m.Release(button)
	m.StepFrame()
	return 2
}

func placementEffectObserved(before, after tetris.State) bool {
	return after.GameOver ||
		after.Complete ||
		after.Board != before.Board ||
		after.LinesCleared != before.LinesCleared ||
		after.LinesRemaining != before.LinesRemaining
}
