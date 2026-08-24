package emu

import (
	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
)

// Button is a joypad button.
type Button = gomeboy.Button

const (
	A      Button = gomeboy.ButtonA
	B      Button = gomeboy.ButtonB
	Start  Button = gomeboy.ButtonStart
	Select Button = gomeboy.ButtonSelect
	Up     Button = gomeboy.ButtonUp
	Down   Button = gomeboy.ButtonDown
	Left   Button = gomeboy.ButtonLeft
	Right  Button = gomeboy.ButtonRight
)

// Tap presses a button for holdFrames, releases it, then steps gapFrames.
// Defaults that are known to work on Pokemon Red: holdFrames=3, gapFrames=7.
func (m *Emu) Tap(b Button, holdFrames, gapFrames int) {
	m.Hold(b, holdFrames)
	m.StepFrames(gapFrames)
}

// Hold presses a button, steps n frames, and releases it.
func (m *Emu) Hold(b Button, n int) {
	m.Press(b)
	m.StepFrames(n)
	m.Release(b)
}

// Press presses a button for callers that manage the hold themselves.
func (m *Emu) Press(b Button) {
	m.e.Press(b)
}

// Release releases a button for callers that manage the hold themselves.
func (m *Emu) Release(b Button) {
	m.e.Release(b)
}
