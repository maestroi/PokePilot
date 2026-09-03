package emu

import (
	"encoding/hex"
	"fmt"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

const checkedStateMagic = "GMBSTATE"

// CPUState is a compact read-only snapshot of the emulated CPU and interrupt
// state. It is for diagnostics and assertions, not gameplay planning.
type CPUState = gomeboy.CPUState

// PPUState is a compact read-only snapshot of PPU timing and display state.
type PPUState = gomeboy.PPUState

// InstructionStep describes one debugger-level CPU step.
type InstructionStep = gomeboy.InstructionStep

// MemoryWatchHit describes the instruction boundary at which an observed byte
// first differed from its value when the watch began.
type MemoryWatchHit struct {
	Address uint16
	Before  byte
	After   byte
	Steps   uint64
	Step    InstructionStep
	CPU     CPUState
	PPU     PPUState
}

// CPUState returns the current CPU registers and interrupt state.
func (m *Emu) CPUState() CPUState {
	return m.e.CPUState()
}

// PPUState returns the current compact PPU state.
func (m *Emu) PPUState() PPUState {
	return m.e.PPUState()
}

// StepInstruction executes one debugger-level CPU step. Unlike StepFrame it
// deliberately does not run PokePilot frame/sample/capture/pacing hooks. Keep
// this on diagnostic paths; gameplay code should continue to use StepFrame or
// StepFrames so those hooks retain their current semantics.
func (m *Emu) StepInstruction() InstructionStep {
	return m.e.StepInstruction()
}

// WatchMemoryChange executes debugger-level CPU steps until addr differs from
// its initial observed value. It has the same diagnostic-only hook caveat as
// StepInstruction.
func (m *Emu) WatchMemoryChange(addr uint16, maxSteps uint64) (MemoryWatchHit, error) {
	before := m.Peek8(addr)
	for i := uint64(0); i < maxSteps; i++ {
		step := m.e.StepInstruction()
		after := m.Peek8(addr)
		if after == before {
			continue
		}
		return MemoryWatchHit{
			Address: addr,
			Before:  before,
			After:   after,
			Steps:   i + 1,
			Step:    step,
			CPU:     m.e.CPUState(),
			PPU:     m.e.PPUState(),
		}, nil
	}
	return MemoryWatchHit{
		Address: addr,
		Before:  before,
		After:   m.Peek8(addr),
		Steps:   maxSteps,
		CPU:     m.e.CPUState(),
		PPU:     m.e.PPUState(),
	}, fmt.Errorf("emu: memory watch %#04x did not change within %d CPU steps", addr, maxSteps)
}

// Cycle returns the emulator's deterministic cycle counter.
func (m *Emu) Cycle() uint64 {
	return m.e.Cycle()
}

// ROMSHA256Hex returns the loaded ROM's SHA-256 fingerprint as lowercase hex.
func (m *Emu) ROMSHA256Hex() string {
	sum := m.e.ROMSHA256()
	return hex.EncodeToString(sum[:])
}

// StateHashHex returns a deterministic execution-state fingerprint. GomeBoy
// intentionally excludes framebuffer output while including execution state.
func (m *Emu) StateHashHex() (string, error) {
	return m.e.StateHashHex()
}

// SaveStateChecked serializes a durable state envelope with ROM/model/build
// identity and payload checksum. Prefer this for evidence that leaves the
// current process; SaveState remains the lower-level raw state primitive.
func (m *Emu) SaveStateChecked() ([]byte, error) {
	return m.e.SaveStateChecked()
}

// LoadStateChecked verifies and restores a state produced by SaveStateChecked.
func (m *Emu) LoadStateChecked(b []byte) error {
	return m.e.LoadStateChecked(b)
}

func isCheckedState(b []byte) bool {
	return len(b) >= len(checkedStateMagic) && string(b[:len(checkedStateMagic)]) == checkedStateMagic
}
