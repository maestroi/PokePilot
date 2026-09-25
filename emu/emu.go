package emu

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

// Emu is a headless Pokemon Red emulator session.
type Emu struct {
	e *gomeboy.Emulator

	// semanticROM is the verified base ROM PokePilot decoders should use.
	// Experiment modes may run a byte-derived cartridge in GomeBoy while the
	// game profile, map parser and battle data keep using this known identity.
	semanticROM []byte

	// Set by Watch. Nil unless a human is watching; see emu/watch.go.
	spec        frameSpectator
	watchRoutes map[string]http.Handler
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
	// samples while a screen is being served. OnSample still fires in
	// headless mode; agent.Run AlsoSamples onto it rather than replacing.
	onFrame func(*Emu)

	// frameDeadline is an absolute frame count temporarily installed by
	// WithFrameDeadline. Zero disables it. StepFrames takes the single-frame
	// path while it is active so a long batched settle cannot jump past the
	// caller's run budget without returning control.
	frameDeadline uint64

	// Set by Pace. Zero means run flat out; see emu/watch.go.
	frameDur  time.Duration
	nextFrame time.Time

	// stepped mirrors FrameCount after each step for Progress readers on
	// other goroutines.
	stepped atomic.Uint64

	// view, when bound, answers Peek*/SnapshotMemory in the running game
	// profile's canonical memory coordinates. See BindMemoryView.
	view MemoryView
}

// MemoryView presents the machine's native memory in another coordinate
// system. A Gen-I revision whose RAM layout differs from the canonical engine
// (Yellow against Red) binds one so shared decoders keep their addresses.
// ReadInto must fill dst from canonical addr using only native reads.
type MemoryView interface {
	ReadInto(native func(addr uint16, dst []byte), addr uint16, dst []byte)
}

// BindMemoryView routes Peek8, Peek16, PeekInto and SnapshotMemory through v.
// A nil v restores native reads. The Peek*Native methods always read the
// machine as it is, for game-owned decoders and forensic dumps.
func (m *Emu) BindMemoryView(v MemoryView) {
	m.view = v
}

// Open loads a ROM using the cartridge-inferred hardware model. It performs
// no other disk I/O. Keep this behavior for diagnostics and historical states
// that were created before production runs moved to CGB hardware.
func Open(romPath string) (*Emu, error) {
	return openModel(romPath, gomeboy.ModelAuto)
}

// OpenCGB loads a ROM as if it were inserted into a Game Boy Color. Original
// DMG games such as Pokemon Red keep their normal game logic while GomeBoy
// applies the CGB's built-in colorization palettes to rendered frames.
func OpenCGB(romPath string) (*Emu, error) {
	return openModel(romPath, gomeboy.ModelCGB)
}

func openModel(romPath string, model gomeboy.Model) (*Emu, error) {
	opts := []gomeboy.Option{
		gomeboy.WithROM(romPath),
		gomeboy.Headless(),
	}
	if model != gomeboy.ModelAuto {
		opts = append(opts, gomeboy.WithModel(model))
	}
	e, err := gomeboy.New(opts...)
	if err != nil {
		return nil, err
	}
	return &Emu{e: e, semanticROM: e.ROM()}, nil
}

// Close releases resources held by the emulator.
func (m *Emu) Close() error {
	return m.e.Close()
}

// StepFrame advances the emulator by exactly one frame.
func (m *Emu) StepFrame() {
	m.e.StepFrame()
	m.stepped.Store(m.e.FrameCount())
	if m.onFrame != nil {
		m.onFrame(m)
	}
	if m.trace == nil && m.onSample != nil {
		// Headless sampling: without Watch there is no sampleTrace to call
		// the sample hook, but a headless run (badgerun) still needs its
		// per-frame RAM sampling — agent.Run's dialogue tape is installed
		// through AlsoSample and is dead code otherwise. In watch mode this
		// is skipped: sampleTrace already calls onSample at capture cadence.
		m.onSample(m)
	}
	m.capture()
	m.throttle(1)
	// Check only after the frame, hooks and capture all completed. The
	// private deadline panic therefore never exposes a half-sampled frame to
	// heartbeat/watch consumers; it merely unwinds the synchronous caller.
	m.checkFrameDeadline()
}

// StepFrames advances the emulator by n frames. With a per-frame hook OR an
// active frame deadline it steps one frame at a time so every frame is visible
// and a synchronous caller can be interrupted exactly at its budget. With no
// hook/deadline it takes the fast batch path, which exists because stepping one
// frame at a time through a long settle is measurably slower.
func (m *Emu) StepFrames(n int) {
	if m.frameDeadline != 0 || m.onFrame != nil || (m.trace == nil && m.onSample != nil) {
		for i := 0; i < n; i++ {
			m.StepFrame()
		}
		return
	}
	m.e.StepFrames(n)
	m.stepped.Store(m.e.FrameCount())
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
	if m.view == nil {
		return m.e.Peek8(addr)
	}
	var b [1]byte
	m.view.ReadInto(m.e.PeekInto, addr, b[:])
	return b[0]
}

// Peek16 reads a little-endian 16-bit value without side effects,
// for CPU-style pointers.
func (m *Emu) Peek16(addr uint16) uint16 {
	if m.view == nil {
		return m.e.Peek16(addr)
	}
	var b [2]byte
	m.view.ReadInto(m.e.PeekInto, addr, b[:])
	return uint16(b[0]) | uint16(b[1])<<8
}

// PeekInto fills dst with len(dst) bytes starting at addr, without
// side effects and without allocating.
func (m *Emu) PeekInto(addr uint16, dst []byte) {
	if m.view == nil {
		m.e.PeekInto(addr, dst)
		return
	}
	m.view.ReadInto(m.e.PeekInto, addr, dst)
}

// SnapshotMemory copies the complete 64 KiB address space into dst and
// returns the frame number the bytes belong to.
func (m *Emu) SnapshotMemory(dst []byte) (uint64, error) {
	if m.view == nil {
		return m.e.SnapshotMemory(dst)
	}
	raw := make([]byte, len(dst))
	frame, err := m.e.SnapshotMemory(raw)
	if err != nil {
		return frame, err
	}
	m.view.ReadInto(func(addr uint16, out []byte) { copy(out, raw[addr:]) }, 0, dst)
	return frame, nil
}

// Peek8Native reads the machine's own byte at addr, ignoring any bound view.
func (m *Emu) Peek8Native(addr uint16) byte {
	return m.e.Peek8(addr)
}

// PeekIntoNative fills dst from the machine's own memory, ignoring any bound
// view.
func (m *Emu) PeekIntoNative(addr uint16, dst []byte) {
	m.e.PeekInto(addr, dst)
}

// SnapshotMemoryNative copies the machine's own 64 KiB address space,
// ignoring any bound view. Forensic dumps use it so a .ram file is always
// the bytes the CPU saw.
func (m *Emu) SnapshotMemoryNative(dst []byte) (uint64, error) {
	return m.e.SnapshotMemory(dst)
}

// ROM returns a caller-owned copy of the semantic/base ROM. Normally this is
// identical to the cartridge GomeBoy is running. Derived experiment ROMs keep
// the verified base here so profile detection and ROM parsers retain the exact
// supported revision identity.
func (m *Emu) ROM() []byte {
	if m.semanticROM != nil {
		return append([]byte(nil), m.semanticROM...)
	}
	return m.e.ROM()
}

// SaveState serializes the emulator's complete execution state in GomeBoy's
// raw format. Use SaveStateChecked for durable evidence outside this process.
func (m *Emu) SaveState() ([]byte, error) {
	return m.e.SaveState()
}

// LoadState restores either a legacy/raw GomeBoy state or a checked durable
// state. This keeps existing fixtures and resumable-run checkpoints readable
// while allowing new diagnostics to use SaveStateChecked.
func (m *Emu) LoadState(b []byte) error {
	var err error
	if isCheckedState(b) {
		err = m.e.LoadStateChecked(b)
	} else {
		err = m.e.LoadState(b)
	}
	if err != nil {
		return err
	}

	// A state restore is an explicit visual epoch boundary. The long-lived
	// farm worker starts Watch before it boots and leases runs, so its spectator
	// queue may still contain intro or previous-run frames. A resumed checkpoint
	// can have a higher frame counter than those frames, which means rollback
	// detection alone cannot notice the boundary. Reset and seed the spectator
	// from the restored machine immediately; preview failures remain diagnostic
	// only and must never turn a valid state restore into a gameplay failure.
	if m.spec != nil {
		_ = m.spec.Reset(m.e)
		m.lastCapture = m.e.FrameCount()
	}
	return nil
}

// Progress is FrameCount as of the last completed step, safe to read from any
// goroutine. A watchdog uses it to tell a slow emulator from one stuck inside
// a single frame.
func (m *Emu) Progress() uint64 {
	return m.stepped.Load()
}

// FrameCount returns the number of frames stepped since the ROM was loaded.
func (m *Emu) FrameCount() uint64 {
	return m.e.FrameCount()
}
