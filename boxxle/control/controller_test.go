package control

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// mockMachine simulates a simple Boxxle board for testing the executor.
// It tracks the player and crate positions and simulates movement when a
// D-pad button is pressed and frames are stepped.
type mockMachine struct {
	player boxxle.Pos
	crate  boxxle.Pos
	walls  map[boxxle.Pos]bool
	// pressed is the currently held D-pad button (zero = none).
	pressed emu.Button
	// stepsSincePress counts frames since the last press.
	stepsSincePress int
}

func newMockMachine(player, crate boxxle.Pos, walls map[boxxle.Pos]bool) *mockMachine {
	if walls == nil {
		walls = map[boxxle.Pos]bool{}
	}
	return &mockMachine{
		player: player,
		crate:  crate,
		walls:  walls,
	}
}

func (m *mockMachine) Peek8(addr uint16) byte { return 0 }
func (m *mockMachine) PeekInto(addr uint16, buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}

func (m *mockMachine) Press(btn emu.Button) {
	m.pressed = btn
	m.stepsSincePress = 0
}

func (m *mockMachine) Release(btn emu.Button) {
	if btn == m.pressed {
		m.pressed = 0
	}
}

func (m *mockMachine) StepFrame() {
	m.stepsSincePress++
	// Move the player on the first frame after a press (simulating the
	// game's movement timing).
	if m.pressed == 0 || m.stepsSincePress > 1 {
		return
	}

	dir := m.pressedToDir()
	if dir == 0 {
		return
	}

	dx, dy := dirDelta(dir)
	target := boxxle.Pos{X: m.player.X + dx, Y: m.player.Y + dy}

	// Check if the target is a wall.
	if m.walls[target] {
		return
	}

	// Check if the target is a crate.
	if target == m.crate {
		// Try to push the crate.
		crateTarget := boxxle.Pos{X: target.X + dx, Y: target.Y + dy}
		if m.walls[crateTarget] {
			return
		}
		if crateTarget == m.player {
			return
		}
		// Push the crate.
		m.crate = crateTarget
	}

	// Move the player.
	m.player = target
}

func (m *mockMachine) pressedToDir() boxxle.Direction {
	switch m.pressed {
	case emu.Up:
		return boxxle.DirUp
	case emu.Down:
		return boxxle.DirDown
	case emu.Left:
		return boxxle.DirLeft
	case emu.Right:
		return boxxle.DirRight
	}
	return 0
}

func dirDelta(d boxxle.Direction) (int, int) {
	switch d {
	case boxxle.DirUp:
		return 0, -1
	case boxxle.DirDown:
		return 0, 1
	case boxxle.DirLeft:
		return -1, 0
	case boxxle.DirRight:
		return 1, 0
	}
	return 0, 0
}

// mockDecoder decodes the current board from the mock machine.
type mockDecoder struct {
	m *mockMachine
}

func (d *mockDecoder) DecodeBoxxleState(_ game.MemoryReader) (boxxle.State, error) {
	return boxxle.State{
		Screen: boxxle.ScreenPuzzle,
		Width:  10,
		Height: 10,
		Crates: []boxxle.Pos{d.m.crate},
		Player: &boxxle.Pos{X: d.m.player.X, Y: d.m.player.Y},
	}, nil
}

func TestPushExecutes(t *testing.T) {
	// Board:
	//   . . . . .
	//   . @ $ . .
	//   . . . . .
	// Player at (1,1), crate at (3,1). Push the crate right.
	player := boxxle.Pos{X: 1, Y: 1}
	crate := boxxle.Pos{X: 3, Y: 1}
	m := newMockMachine(player, crate, nil)
	dec := &mockDecoder{m: m}

	push := boxxle.LegalPush{
		Crate:      crate,
		Dir:        boxxle.DirRight,
		PlayerFrom: boxxle.Pos{X: 2, Y: 1},
		CrateTo:    boxxle.Pos{X: 4, Y: 1},
	}

	res, err := Push(m, dec, push)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if !res.CrateMoved {
		t.Error("expected crate to have moved")
	}
	if m.crate != (boxxle.Pos{X: 4, Y: 1}) {
		t.Errorf("expected crate at (4,1), got %v", m.crate)
	}
	if m.player != (boxxle.Pos{X: 3, Y: 1}) {
		t.Errorf("expected player at (3,1), got %v", m.player)
	}
}

func TestPushBlockedByWall(t *testing.T) {
	// Board:
	//   . . . # .
	//   . @ $ . .
	//   . . . . .
	// Player at (1,1), crate at (3,1). Pushing the crate right is blocked
	// by the wall at (4,1).
	player := boxxle.Pos{X: 1, Y: 1}
	crate := boxxle.Pos{X: 3, Y: 1}
	walls := map[boxxle.Pos]bool{
		{X: 4, Y: 1}: true,
	}
	m := newMockMachine(player, crate, walls)
	dec := &mockDecoder{m: m}

	push := boxxle.LegalPush{
		Crate:      crate,
		Dir:        boxxle.DirRight,
		PlayerFrom: boxxle.Pos{X: 2, Y: 1},
		CrateTo:    boxxle.Pos{X: 4, Y: 1},
	}

	_, err := Push(m, dec, push)
	if err == nil {
		t.Fatal("expected push to be blocked by wall")
	}
	if !errors.Is(err, ErrBlocked) {
		t.Errorf("expected ErrBlocked, got %v", err)
	}
}

func TestPushWalkBlocked(t *testing.T) {
	// Board:
	//   . . # . .
	//   . @ $ . .
	//   . . . . .
	// Player at (1,1), crate at (3,1). The wall at (2,1) blocks the player
	// from reaching the pre-push square (2,1).
	player := boxxle.Pos{X: 1, Y: 1}
	crate := boxxle.Pos{X: 3, Y: 1}
	walls := map[boxxle.Pos]bool{
		{X: 2, Y: 1}: true,
	}
	m := newMockMachine(player, crate, walls)
	dec := &mockDecoder{m: m}

	push := boxxle.LegalPush{
		Crate:      crate,
		Dir:        boxxle.DirRight,
		PlayerFrom: boxxle.Pos{X: 2, Y: 1},
		CrateTo:    boxxle.Pos{X: 4, Y: 1},
	}

	_, err := Push(m, dec, push)
	if err == nil {
		t.Fatal("expected walk to be blocked by wall")
	}
	if !errors.Is(err, ErrBlocked) {
		t.Errorf("expected ErrBlocked, got %v", err)
	}
}
