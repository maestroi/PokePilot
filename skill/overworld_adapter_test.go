package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

const (
	fakeOverworldMap uint16 = 160 + iota
	fakeOverworldX
	fakeOverworldY
	fakeOverworldFlags
)

const (
	fakeOverworldIdle = 1 << iota
	fakeOverworldBattle
	fakeOverworldDialogue
	fakeOverworldControllable
)

type fakeGen2OverworldDecoder struct{}

func (fakeGen2OverworldDecoder) DecodeOverworld(r game.MemoryReader) game.OverworldState {
	flags := r.Peek8(fakeOverworldFlags)
	return game.OverworldState{
		NativeMapID:  uint16(r.Peek8(fakeOverworldMap)),
		X:            r.Peek8(fakeOverworldX),
		Y:            r.Peek8(fakeOverworldY),
		Controllable: flags&fakeOverworldControllable != 0,
		MovementIdle: flags&fakeOverworldIdle != 0,
		InBattle:     flags&fakeOverworldBattle != 0,
		InDialogue:   flags&fakeOverworldDialogue != 0,
	}
}

type fakeOverworldMachine struct {
	mem       [256]byte
	held      emu.Button
	move      bool
	stepCount int
}

func (m *fakeOverworldMachine) Peek8(addr uint16) byte { return m.mem[addr] }

func (m *fakeOverworldMachine) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):])
}

func (m *fakeOverworldMachine) Press(btn emu.Button) {
	m.held = btn
	m.mem[fakeOverworldFlags] &^= fakeOverworldIdle
}

func (m *fakeOverworldMachine) Release(btn emu.Button) {
	if m.held == btn {
		m.held = 0
	}
	m.mem[fakeOverworldFlags] |= fakeOverworldIdle
}

func (m *fakeOverworldMachine) StepFrame() {
	m.stepCount++
	if !m.move || m.held == 0 {
		return
	}
	switch m.held {
	case emu.Up:
		m.mem[fakeOverworldY]--
	case emu.Down:
		m.mem[fakeOverworldY]++
	case emu.Left:
		m.mem[fakeOverworldX]--
	case emu.Right:
		m.mem[fakeOverworldX]++
	}
	// One tile per requested press is enough to model the semantic contract.
	m.move = false
}

func TestGenericStepOnceUsesFakeGen2OverworldState(t *testing.T) {
	m := &fakeOverworldMachine{move: true}
	m.mem[fakeOverworldMap] = 7
	m.mem[fakeOverworldX] = 20
	m.mem[fakeOverworldY] = 11
	m.mem[fakeOverworldFlags] = fakeOverworldIdle | fakeOverworldControllable

	if err := stepOnceWithOverworldDecoder(m, world.StepRight, fakeGen2OverworldDecoder{}); err != nil {
		t.Fatalf("step right: %v", err)
	}
	if got := m.mem[fakeOverworldX]; got != 21 {
		t.Fatalf("x = %d, want 21", got)
	}
	if got := m.mem[fakeOverworldY]; got != 11 {
		t.Fatalf("y = %d, want 11", got)
	}
	if m.held != 0 {
		t.Fatalf("button remained held: %v", m.held)
	}
}

func TestGenericStepOnceReportsBlockedFromSemanticCoordinates(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldX] = 4
	m.mem[fakeOverworldY] = 9
	m.mem[fakeOverworldFlags] = fakeOverworldIdle | fakeOverworldControllable

	err := stepOnceWithOverworldDecoder(m, world.StepUp, fakeGen2OverworldDecoder{})
	var blocked *ErrBlocked
	if !errors.As(err, &blocked) {
		t.Fatalf("error = %v, want ErrBlocked", err)
	}
	if blocked.At.X != 4 || blocked.At.Y != 9 {
		t.Fatalf("blocked at (%d,%d), want (4,9)", blocked.At.X, blocked.At.Y)
	}
}

func TestGenericMovementInterruptionUsesSemanticBattleState(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldFlags] = fakeOverworldBattle
	if err := movementInterruptionWithDecoder(m, fakeGen2OverworldDecoder{}); !errors.Is(err, ErrBattleInterrupted) {
		t.Fatalf("error = %v, want ErrBattleInterrupted", err)
	}
}

func TestGenericMovementInterruptionUsesSemanticDialogueState(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldFlags] = fakeOverworldDialogue
	if err := movementInterruptionWithDecoder(m, fakeGen2OverworldDecoder{}); !errors.Is(err, ErrDialogueInterrupted) {
		t.Fatalf("error = %v, want ErrDialogueInterrupted", err)
	}
}
