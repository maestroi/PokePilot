package skill

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestCurrentWorldUsesFakeGen2OverworldDecoder(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldMap] = 0x6b
	m.mem[fakeOverworldX] = 41
	m.mem[fakeOverworldY] = 22

	got, err := currentWorldWithDecoder(m, fakeGen2OverworldDecoder{})
	if err != nil {
		t.Fatalf("current world: %v", err)
	}
	want := (Replan{Map: 0x6b, X: 41, Y: 22})
	if got != want {
		t.Fatalf("current world = %+v, want %+v", got, want)
	}
}

func TestCurrentWorldRejectsWideNativeMap(t *testing.T) {
	decoder := fixedOverworldDecoder{state: game.OverworldState{NativeMapID: 0x1ff, X: 4, Y: 8}}
	_, err := currentWorldWithDecoder(&fakeOverworldMachine{}, decoder)
	if err == nil || !strings.Contains(err.Error(), "exceeds current routing range") {
		t.Fatalf("error = %v, want routing-range error", err)
	}
}

type fakeTravelWorldMachine struct {
	mem      [256]byte
	frames   int
	changeAt int
	mapAfter byte
	xAfter   byte
	yAfter   byte
}

func (m *fakeTravelWorldMachine) Peek8(addr uint16) byte { return m.mem[addr] }

func (m *fakeTravelWorldMachine) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):])
}

func (m *fakeTravelWorldMachine) StepFrame() {
	m.frames++
	if m.changeAt > 0 && m.frames == m.changeAt {
		m.mem[fakeOverworldMap] = m.mapAfter
		m.mem[fakeOverworldX] = m.xAfter
		m.mem[fakeOverworldY] = m.yAfter
	}
}

func TestSettleWorldWaitsForSemanticMapChangeAfterLoss(t *testing.T) {
	m := &fakeTravelWorldMachine{changeAt: 3, mapAfter: 0x44, xAfter: 3, yAfter: 7}
	m.mem[fakeOverworldMap] = 0x0c
	m.mem[fakeOverworldX] = 5
	m.mem[fakeOverworldY] = 6
	m.mem[fakeOverworldFlags] = fakeOverworldIdle | fakeOverworldControllable

	got, err := settleWorldWithDecoder(m, fakeGen2OverworldDecoder{}, Replan{Map: 0x0c, X: 5, Y: 6}, true)
	if err != nil {
		t.Fatalf("settle world: %v", err)
	}
	want := (Replan{Map: 0x44, X: 3, Y: 7})
	if got != want {
		t.Fatalf("settled world = %+v, want %+v", got, want)
	}
	if m.frames < 3+worldStableFrames {
		t.Fatalf("settled after %d frames, want map change plus %d stable frames", m.frames, worldStableFrames)
	}
}
