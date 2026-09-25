package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

type semanticHealBoundaryClock struct {
	mem     state.Mem
	steps   int
	readyAt int
}

func (m *semanticHealBoundaryClock) Peek8(addr uint16) byte {
	return m.mem[addr]
}

func (m *semanticHealBoundaryClock) PeekInto(addr uint16, dst []byte) {
	src := m.mem[addr:]
	if len(dst) > len(src) {
		dst = dst[:len(src)]
	}
	copy(dst, src)
}

func (*semanticHealBoundaryClock) Tap(emu.Button, int, int) {}

func (m *semanticHealBoundaryClock) StepFrame() {
	m.steps++
	if m.steps >= m.readyAt {
		m.mem[fakeOverworldFlags] |= fakeOverworldControllable
	}
}

func (m *semanticHealBoundaryClock) StepFrames(n int) {
	for i := 0; i < n; i++ {
		m.StepFrame()
	}
}

func TestHealBoundaryUsesSemanticControllability(t *testing.T) {
	m := &semanticHealBoundaryClock{readyAt: 5}
	m.mem[fakeOverworldMap] = 0x44
	m.mem[fakeOverworldX] = 3
	m.mem[fakeOverworldY] = 3

	if err := settleHealBoundaryWithDecoder(m, fakeGen2OverworldDecoder{}, 500); err != nil {
		t.Fatalf("settle semantic heal boundary: %v", err)
	}
	minSteps := m.readyAt + healBoundaryStableFrames - 1
	if m.steps < minSteps {
		t.Fatalf("steps = %d, want at least %d so semantic controllability was stable for the full boundary", m.steps, minSteps)
	}
}
