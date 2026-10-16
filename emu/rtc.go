package emu

import (
	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

// RTCState is an immutable copy of the live MBC3 clock from emulated cycles.
type RTCState = gomeboy.RTCState

var (
	ErrRTCUnavailable = gomeboy.ErrRTCUnavailable
	ErrRTCHalted      = gomeboy.ErrRTCHalted
)

// RTC exposes the live cartridge time without reading the host clock or
// changing the CPU-visible latched registers. Clockless ROMs return a typed error.
func (m *Emu) RTC() (RTCState, error) {
	if m == nil || m.e == nil {
		return RTCState{}, ErrRTCUnavailable
	}
	return m.e.RTC()
}

// AdvanceRTCSeconds is an explicit emulation event, not simulated CPU frames.
// State/checkpoints capture its result together with the current frame count.
// Call only on the emulator stepping goroutine, like StepFrame.
func (m *Emu) AdvanceRTCSeconds(seconds uint32) error {
	if m == nil || m.e == nil {
		return ErrRTCUnavailable
	}
	return m.e.AdvanceRTCSeconds(seconds)
}
