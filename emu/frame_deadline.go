package emu

import (
	"errors"
	"fmt"
)

// ErrFrameDeadline is returned by WithFrameDeadline when emulator execution
// reaches the supplied absolute frame while fn is still running. It exists so
// a caller can bound an entire synchronous objective, including loops deep in
// skill code that would otherwise prevent the caller from regaining control
// to check its budget.
var ErrFrameDeadline = errors.New("emu: frame deadline reached")

type frameDeadlinePanic struct {
	frame    uint64
	deadline uint64
}

// WithFrameDeadline runs fn while enforcing an absolute emulator-frame
// deadline. Emulator stepping remains synchronous, so the deadline is checked
// at the end of every fully stepped frame; a private panic is used only to
// unwind arbitrary nested skill code back to this boundary. Any unrelated
// panic is re-thrown unchanged.
//
// Nested calls preserve the earlier deadline. A zero deadline disables the
// guard for this call.
func (m *Emu) WithFrameDeadline(deadline uint64, fn func() error) (err error) {
	if deadline == 0 {
		return fn()
	}
	if now := m.FrameCount(); now >= deadline {
		return fmt.Errorf("%w: frame %d, limit %d", ErrFrameDeadline, now, deadline)
	}

	previous := m.frameDeadline
	if previous == 0 || deadline < previous {
		m.frameDeadline = deadline
	}
	defer func() {
		m.frameDeadline = previous
		if r := recover(); r != nil {
			if hit, ok := r.(frameDeadlinePanic); ok {
				err = fmt.Errorf("%w: frame %d, limit %d", ErrFrameDeadline, hit.frame, hit.deadline)
				return
			}
			panic(r)
		}
	}()
	return fn()
}

func (m *Emu) checkFrameDeadline() {
	if m.frameDeadline == 0 {
		return
	}
	if frame := m.FrameCount(); frame >= m.frameDeadline {
		panic(frameDeadlinePanic{frame: frame, deadline: m.frameDeadline})
	}
}
