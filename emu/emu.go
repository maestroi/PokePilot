package emu

import (
	"fmt"
	"time"

	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
)

const addressSpaceSize = 1 << 16

// Emu is a headless Pokemon Red emulator session.
type Emu struct {
	e *gomeboy.Emulator

	// Set by Watch. Nil unless a human is watching; see emu/watch.go.
	spec        *gomeboy.Spectator
	specEvery   int
	lastCapture uint64

	// Set by Watch alongside spec. trace is nil unless watching; traceSt
	// holds the previous sample for change detection. See emu/trace.go.
	onSample func(*Emu)
	trace    *traceBuf
	traceSt  traceState

	// onFrame runs after every stepped frame, on the goroutine stepping the
	// emulator. Headless consumers use it for per-frame RAM sampling (for
	// example counting battle-flag transitions): Watch's sampleTrace only
	// samples while a screen is being served, and OnSample alone is not
	// enough because agent.Run replaces it with its own hook.
	onFrame func(*Emu)

	// Set by Pace. Zero means run flat out; see emu/watch.go.
	frameDur  time.Duration
	nextFrame time.Time
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
	if m.onFrame != nil {
		m.onFrame(m)
	}
	if m.trace == nil && m.onSample != nil {
		// Headless sampling: without Watch there is no sampleTrace to call
		// the sample hook, but a headless run (badgerun) still needs its
		// per-frame RAM sampling — agent.Run's dialogue tape is installed
		// through OnSample and is dead code otherwise. In watch mode this
		// is skipped: sampleTrace already calls onSample at capture cadence.
		m.onSample(m)
	}
	m.capture()
	m.throttle(1)
}

// StepFrames advances the emulator by n frames. With a per-frame hook
// installed it steps one frame at a time so the hook sees every frame —
// skill.Talk pages whole conversations through here, and a batched call
// would be invisible to it. The condition must match StepFrame's own two
// hook conditions exactly: if they drift, some frames sample and some do
// not. With no hook it takes the fast batch path, which exists because
// stepping one frame at a time through a long settle is measurably slower.
func (m *Emu) StepFrames(n int) {
	if m.onFrame != nil || (m.trace == nil && m.onSample != nil) {
		for i := 0; i < n; i++ {
			m.StepFrame()
		}
		return
	}
	m.e.StepFrames(n)
	m.capture()
	m.throttle(n)
}

// OnFrame registers fn to run after every frame step, on the goroutine
// stepping the emulator. It is for headless consumers that need per-frame
// RAM sampling without Watch (which serves a screen and an HTTP server).
// Like OnSample, fn runs where the emulator steps: it may read memory but
// must not step the emulator.
func (m *Emu) OnFrame(fn func(*Emu)) {
	m.onFrame = fn
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

// SnapshotMemory copies the complete 64 KiB address space into dst and
// returns the frame number the bytes belong to. PokePilot composes this from
// GomeBoy's existing side-effect-free PeekInto and FrameCount primitives so
// the forensic feature does not depend on an unmerged emulator revision.
func (m *Emu) SnapshotMemory(dst []byte) (uint64, error) {
	if len(dst) != addressSpaceSize {
		return 0, fmt.Errorf("emu: SnapshotMemory: buffer is %d bytes, want %d", len(dst), addressSpaceSize)
	}
	m.e.PeekInto(0, dst)
	return m.e.FrameCount(), nil
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
