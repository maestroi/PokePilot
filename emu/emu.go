package emu

import (
	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
)

// Emu is a headless Pokemon Red emulator session.
type Emu struct {
	e *gomeboy.Emulator

	// Set by Watch. Nil unless a human is watching; see emu/watch.go.
	spec        *gomeboy.Spectator
	specEvery   int
	lastCapture uint64
}

// Open loads a ROM from disk. It performs no other disk I/O.
func Open(romPath string) (*Emu, error) {
	e, err := gomeboy.New(gomeboy.WithROM(romPath), gomeboy.Headless())
	if err != nil {
		return nil, err
	}
	return &Emu{e: e}, nil
}

// Close releases resources held by the emulator.
func (m *Emu) Close() error {
	return m.e.Close()
}

// StepFrame advances the emulator by exactly one frame.
func (m *Emu) StepFrame() {
	m.e.StepFrame()
	m.capture()
}

// StepFrames advances the emulator by n frames.
func (m *Emu) StepFrames(n int) {
	m.e.StepFrames(n)
	m.capture()
}

// Peek8 reads a byte without any hardware side effects.
func (m *Emu) Peek8(addr uint16) byte {
	return m.e.Peek8(addr)
}

// Peek16 reads a little-endian 16-bit value without side effects,
// for CPU-style pointers.
func (m *Emu) Peek16(addr uint16) uint16 {
	return m.e.Peek16(addr)
}

// PeekInto fills dst with len(dst) bytes starting at addr, without side
// effects and without allocating.
func (m *Emu) PeekInto(addr uint16, dst []byte) {
	m.e.PeekInto(addr, dst)
}

// ROM returns the bytes of the loaded ROM. The slice aliases emulator
// memory and must not be modified.
func (m *Emu) ROM() []byte {
	return m.e.ROM()
}

// SaveState serializes the emulator's complete execution state.
func (m *Emu) SaveState() ([]byte, error) {
	return m.e.SaveState()
}

// LoadState restores a state previously produced by SaveState.
func (m *Emu) LoadState(b []byte) error {
	return m.e.LoadState(b)
}

// FrameCount returns the number of frames stepped since the ROM was loaded.
func (m *Emu) FrameCount() uint64 {
	return m.e.FrameCount()
}
