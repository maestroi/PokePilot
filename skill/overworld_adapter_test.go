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
	fakeOverworldFacing
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
	facing := ""
	switch r.Peek8(fakeOverworldFacing) {
	case 1:
		facing = "up"
	case 2:
		facing = "down"
	case 3:
		facing = "left"
	case 4:
		facing = "right"
	}
	return game.OverworldState{
		NativeMapID:  uint16(r.Peek8(fakeOverworldMap)),
		X:            r.Peek8(fakeOverworldX),
		Y:            r.Peek8(fakeOverworldY),
		Facing:       facing,
		Controllable: flags&fakeOverworldControllable != 0,
		MovementIdle: flags&fakeOverworldIdle != 0,
		InBattle:     flags&fakeOverworldBattle != 0,
		InDialogue:   flags&fakeOverworldDialogue != 0,
	}
}

type fakeOverworldMachine struct {
	mem          [256]byte
	held         emu.Button
	move         bool
	stepCount    int
	battleAtStep int
	// lostControlAtStep models an encounter that clears control before it
	// enters battle mode.
	lostControlAtStep int
}

func (m *fakeOverworldMachine) Peek8(addr uint16) byte { return m.mem[addr] }

func (m *fakeOverworldMachine) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):])
}

func (m *fakeOverworldMachine) Press(btn emu.Button) {
	m.held = btn
	m.mem[fakeOverworldFlags] &^= fakeOverworldIdle
	switch btn {
	case emu.Up:
		m.mem[fakeOverworldFacing] = 1
	case emu.Down:
		m.mem[fakeOverworldFacing] = 2
	case emu.Left:
		m.mem[fakeOverworldFacing] = 3
	case emu.Right:
		m.mem[fakeOverworldFacing] = 4
	}
}

func (m *fakeOverworldMachine) Release(btn emu.Button) {
	if m.held == btn {
		m.held = 0
	}
	m.mem[fakeOverworldFlags] |= fakeOverworldIdle
}

func (m *fakeOverworldMachine) StepFrame() {
	m.stepCount++
	if m.lostControlAtStep > 0 && m.stepCount == m.lostControlAtStep {
		m.mem[fakeOverworldFlags] &^= fakeOverworldControllable
	}
	if m.battleAtStep > 0 && m.stepCount >= m.battleAtStep {
		m.mem[fakeOverworldFlags] |= fakeOverworldBattle
	}
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

func TestGenericFaceReportsBattleBeforeFacingSuccess(t *testing.T) {
	m := &fakeOverworldMachine{battleAtStep: 1}
	m.mem[fakeOverworldMap] = 0x0c
	m.mem[fakeOverworldX] = 5
	m.mem[fakeOverworldY] = 25
	m.mem[fakeOverworldFlags] = fakeOverworldIdle | fakeOverworldControllable

	err := faceWithOverworldDecoder(m, fakeGen2OverworldDecoder{}, 5, 24)
	if !errors.Is(err, ErrBattle) {
		t.Fatalf("face error = %v, want ErrBattle when encounter starts during turn tap", err)
	}
}

// The turn registers while a wild encounter has already taken control but has
// not entered battle mode yet; an A press then would land in the battle intro.
func TestGenericFaceReportsBattlePendingAfterFacing(t *testing.T) {
	m := &fakeOverworldMachine{lostControlAtStep: 1, battleAtStep: 60}
	m.mem[fakeOverworldMap] = 0x0c
	m.mem[fakeOverworldX] = 15
	m.mem[fakeOverworldY] = 28
	m.mem[fakeOverworldFlags] = fakeOverworldIdle | fakeOverworldControllable

	err := faceWithOverworldDecoder(m, fakeGen2OverworldDecoder{}, 15, 29)
	if !errors.Is(err, ErrBattle) {
		t.Fatalf("face error = %v, want ErrBattle when an encounter is pending after the turn", err)
	}
}

func TestGenericFaceWithoutControlKeepsFacingPromise(t *testing.T) {
	m := &fakeOverworldMachine{lostControlAtStep: 1}
	m.mem[fakeOverworldMap] = 7
	m.mem[fakeOverworldX] = 20
	m.mem[fakeOverworldY] = 11
	m.mem[fakeOverworldFlags] = fakeOverworldIdle | fakeOverworldControllable

	if err := faceWithOverworldDecoder(m, fakeGen2OverworldDecoder{}, 20, 10); err != nil {
		t.Fatalf("face up without control and without battle: %v", err)
	}
}

func TestGenericFaceUsesSemanticProfileFacing(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeOverworldMap] = 7
	m.mem[fakeOverworldX] = 20
	m.mem[fakeOverworldY] = 11
	m.mem[fakeOverworldFlags] = fakeOverworldIdle | fakeOverworldControllable

	if err := faceWithOverworldDecoder(m, fakeGen2OverworldDecoder{}, 20, 10); err != nil {
		t.Fatalf("face up: %v", err)
	}
	if got := (fakeGen2OverworldDecoder{}).DecodeOverworld(m).Facing; got != "up" {
		t.Fatalf("facing = %q, want up", got)
	}
	if got := m.mem[fakeOverworldX]; got != 20 {
		t.Fatalf("x = %d, want 20", got)
	}
	if got := m.mem[fakeOverworldY]; got != 11 {
		t.Fatalf("y = %d, want 11", got)
	}
}
