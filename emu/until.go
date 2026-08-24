package emu

import "fmt"

// ErrTimeout is returned when a predicate did not become true within the budget.
type ErrTimeout struct {
	Frames int
}

func (e *ErrTimeout) Error() string {
	return fmt.Sprintf("predicate not satisfied within %d frames", e.Frames)
}

// StepUntil advances the emulator one frame at a time until pred returns
// true, or until budgetFrames have elapsed. It returns the number of frames
// stepped, or an *ErrTimeout. pred must not modify the emulator.
//
// StepUntil checks pred before the first step, so an already-satisfied
// condition costs zero frames.
func (m *Emu) StepUntil(budgetFrames int, pred func(*Emu) bool) (int, error) {
	for stepped := 0; stepped < budgetFrames; stepped++ {
		if pred(m) {
			return stepped, nil
		}
		m.StepFrame()
	}
	return budgetFrames, &ErrTimeout{Frames: budgetFrames}
}

// HoldUntil holds a button while waiting for pred, releasing it before it
// returns either way. Used for walking: hold a direction until the tile
// coordinate changes.
func (m *Emu) HoldUntil(b Button, budgetFrames int, pred func(*Emu) bool) (int, error) {
	m.Press(b)
	defer m.Release(b)
	return m.StepUntil(budgetFrames, pred)
}
