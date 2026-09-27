package control

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/tetris"
	tetrisprofile "github.com/maestroi/pokepilot/tetris/profile"
	"github.com/maestroi/pokepilot/tetris/sym"
)

type fakeMachine struct {
	mem [1 << 16]byte

	pressed  map[emu.Button]bool
	previous map[emu.Button]bool
	history  []emu.Button

	blockRotation bool
	blockShift    bool
	neverLock     bool
	lineClear     bool

	lockCountdown int
}

func newFakeMachine() *fakeMachine {
	m := &fakeMachine{
		pressed:  make(map[emu.Button]bool),
		previous: make(map[emu.Button]bool),
	}
	for y := 0; y < tetris.BoardHeight; y++ {
		row := sym.BoardTopLeft + uint16(y)*sym.BoardStride
		for x := 0; x < tetris.BoardWidth; x++ {
			m.mem[row+uint16(x)] = sym.EmptyBoardTile
		}
	}
	m.mem[sym.GameState] = 0x00
	m.mem[sym.GameType] = sym.GameTypeA
	m.mem[sym.ActiveVisible] = 0x00
	m.mem[sym.ActiveX] = 0x3F
	m.mem[sym.ActiveY] = 0x18
	m.mem[sym.ActivePiece] = 0x18  // T, rotation 0
	m.mem[sym.PreviewPiece] = 0x08 // I, rotation 0
	return m
}

func (m *fakeMachine) Peek8(addr uint16) byte {
	return m.mem[addr]
}

func (m *fakeMachine) PeekInto(addr uint16, dst []byte) {
	for i := range dst {
		dst[i] = m.mem[addr+uint16(i)]
	}
}

func (m *fakeMachine) Press(button emu.Button) {
	m.pressed[button] = true
	m.history = append(m.history, button)
}

func (m *fakeMachine) Release(button emu.Button) {
	m.pressed[button] = false
}

func (m *fakeMachine) StepFrame() {
	wasLocking := m.mem[sym.LockStage] != 0
	if wasLocking {
		m.advanceLock()
		m.rememberButtons()
		return
	}

	if m.edge(emu.A) && !m.blockRotation {
		raw := m.mem[sym.ActivePiece]
		rotation := raw & 3
		if rotation == 0 {
			rotation = 3
		} else {
			rotation--
		}
		m.mem[sym.ActivePiece] = raw&^3 | rotation
	}
	if m.edge(emu.B) && !m.blockRotation {
		raw := m.mem[sym.ActivePiece]
		m.mem[sym.ActivePiece] = raw&^3 | ((raw + 1) & 3)
	}
	if m.edge(emu.Left) && !m.blockShift {
		m.mem[sym.ActiveX] -= 8
	}
	if m.edge(emu.Right) && !m.blockShift {
		m.mem[sym.ActiveX] += 8
	}

	if m.pressed[emu.Down] && !m.neverLock {
		m.beginLock()
	}

	m.rememberButtons()
}

func (m *fakeMachine) edge(button emu.Button) bool {
	return m.pressed[button] && !m.previous[button]
}

func (m *fakeMachine) rememberButtons() {
	for _, button := range []emu.Button{
		emu.A, emu.B, emu.Start, emu.Select,
		emu.Up, emu.Down, emu.Left, emu.Right,
	} {
		m.previous[button] = m.pressed[button]
	}
}

func (m *fakeMachine) beginLock() {
	column := 5 + (int(m.mem[sym.ActiveX])-0x3F)/8
	if column >= 0 && column < tetris.BoardWidth && !m.lineClear {
		m.mem[sym.BoardTopLeft+17*sym.BoardStride+uint16(column)] = 0x80
	}
	if m.lineClear {
		m.mem[sym.Lines] = 0x01
	}
	m.mem[sym.LockStage] = 1
	m.mem[sym.ActiveVisible] = 0x80
	m.lockCountdown = 2
}

func (m *fakeMachine) advanceLock() {
	m.lockCountdown--
	if m.lockCountdown > 0 {
		return
	}
	m.mem[sym.LockStage] = 0
	m.mem[sym.ActiveVisible] = 0
	m.mem[sym.ActiveX] = 0x3F
	m.mem[sym.ActiveY] = 0x18
	m.mem[sym.ActivePiece] = m.mem[sym.PreviewPiece]
	m.mem[sym.PreviewPiece] = 0x04 // J
}

func TestPlaceRotatesShiftsDropsAndVerifies(t *testing.T) {
	m := newFakeMachine()

	result, err := Place(tetrisprofile.New(), m, Placement{
		Rotation: 3,
		Column:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Piece != tetris.PieceT {
		t.Fatalf("piece = %s", result.Piece)
	}
	if result.Target.Rotation != 3 || result.Target.Column != 2 {
		t.Fatalf("target = %#v", result.Target)
	}
	if !result.BoardChanged {
		t.Fatal("expected locked board to change")
	}
	if !result.After.ReadyForPieceInput || result.After.Active == nil {
		t.Fatalf("after = %#v", result.After)
	}
	if result.After.Active.Piece != tetris.PieceI {
		t.Fatalf("next active piece = %s", result.After.Active.Piece)
	}
	if !containsButton(m.history, emu.A) || countButton(m.history, emu.Left) != 3 || !containsButton(m.history, emu.Down) {
		t.Fatalf("input history = %#v", m.history)
	}
}

func TestPlaceUsesShortestCounterClockwiseRotation(t *testing.T) {
	m := newFakeMachine()

	if _, err := Place(tetrisprofile.New(), m, Placement{Rotation: 1, Column: 4}); err != nil {
		t.Fatal(err)
	}
	if len(m.history) == 0 || m.history[0] != emu.B {
		t.Fatalf("first input = %#v, want B", m.history)
	}
	if containsButton(m.history, emu.A) {
		t.Fatalf("unexpected clockwise rotation input: %#v", m.history)
	}
}

func TestPlaceRejectsBlockedRotationBeforeDrop(t *testing.T) {
	m := newFakeMachine()
	m.blockRotation = true
	m.mem[sym.Score] = 0x41 // packed BCD 41: a run that has already scored

	result, err := Place(tetrisprofile.New(), m, Placement{Rotation: 1, Column: 4})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("error = %v, want ErrBlocked", err)
	}
	if containsButton(m.history, emu.Down) {
		t.Fatalf("controller dropped after rejected rotation: %#v", m.history)
	}
	assertLiveAbortState(t, result)
}

func TestPlaceRejectsBlockedShiftBeforeDrop(t *testing.T) {
	m := newFakeMachine()
	m.blockShift = true
	m.mem[sym.Score] = 0x41

	result, err := Place(tetrisprofile.New(), m, Placement{Rotation: 0, Column: 3})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("error = %v, want ErrBlocked", err)
	}
	if containsButton(m.history, emu.Down) {
		t.Fatalf("controller dropped after rejected shift: %#v", m.history)
	}
	assertLiveAbortState(t, result)
}

// assertLiveAbortState pins that an aborted placement reports where the game
// actually is. The zero State here used to reach the farm as "score 0, lines 0"
// for a run whose final frame showed SCORE 41.
func assertLiveAbortState(t *testing.T, result Result) {
	t.Helper()
	if result.After.Score != 41 {
		t.Fatalf("after score = %d, want the live 41", result.After.Score)
	}
	if result.After.Active == nil || result.After.Active.Piece != tetris.PieceT {
		t.Fatalf("after active = %#v, want the live piece", result.After.Active)
	}
	if !result.After.ReadyForPieceInput {
		t.Fatal("after must be the live ready state, not the zero State")
	}
	if result.BoardChanged || result.LinesCleared != 0 {
		t.Fatalf("aborted placement changed the board: changed=%v lines=%d", result.BoardChanged, result.LinesCleared)
	}
}

func TestPlaceRequiresReadyPiece(t *testing.T) {
	m := newFakeMachine()
	m.mem[sym.LockStage] = 1

	_, err := Place(tetrisprofile.New(), m, Placement{Rotation: 0, Column: 4})
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("error = %v, want ErrNotReady", err)
	}
	if len(m.history) != 0 {
		t.Fatalf("unexpected input while not ready: %#v", m.history)
	}
}

func TestPlaceTimesOutWhenSoftDropNeverLocks(t *testing.T) {
	m := newFakeMachine()
	m.neverLock = true

	_, err := Place(tetrisprofile.New(), m, Placement{Rotation: 0, Column: 4})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v, want ErrTimeout", err)
	}
	if !containsButton(m.history, emu.Down) {
		t.Fatalf("missing soft-drop input: %#v", m.history)
	}
	if m.pressed[emu.Down] {
		t.Fatal("Down remained pressed after timeout")
	}
}

func TestPlaceAcceptsLineProgressAsVerifiedLockEffect(t *testing.T) {
	m := newFakeMachine()
	m.lineClear = true

	result, err := Place(tetrisprofile.New(), m, Placement{Rotation: 0, Column: 4})
	if err != nil {
		t.Fatal(err)
	}
	if result.BoardChanged {
		t.Fatal("fixture intentionally leaves board unchanged")
	}
	if result.LinesCleared != 1 {
		t.Fatalf("lines cleared = %d, want 1", result.LinesCleared)
	}
}

func TestPlaceValidatesTarget(t *testing.T) {
	for _, target := range []Placement{
		{Rotation: 4, Column: 4},
		{Rotation: 0, Column: -1},
		{Rotation: 0, Column: tetris.BoardWidth},
	} {
		m := newFakeMachine()
		_, err := Place(tetrisprofile.New(), m, target)
		if !errors.Is(err, ErrBlocked) {
			t.Fatalf("target %#v error = %v, want ErrBlocked", target, err)
		}
		if len(m.history) != 0 {
			t.Fatalf("target %#v sent input: %#v", target, m.history)
		}
	}
}

func containsButton(history []emu.Button, want emu.Button) bool {
	return countButton(history, want) != 0
}

func countButton(history []emu.Button, want emu.Button) int {
	count := 0
	for _, button := range history {
		if button == want {
			count++
		}
	}
	return count
}
