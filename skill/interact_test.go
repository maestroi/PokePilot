package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// gridFor builds the collision grid for one map from the live ROM.
func gridFor(t *testing.T, romData []byte, mapID uint8) *world.Grid {
	t.Helper()
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		t.Fatalf("ParseMap(%02x): %v", mapID, err)
	}
	g, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("world.Build(%02x): %v", mapID, err)
	}
	return g
}

func controllable(t *testing.T, m *emu.Emu) bool {
	t.Helper()
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.Controllable(&mem)
}

// TestFaceEachDirection faces all four adjacent tiles of the fixture start
// in turn and checks the decoded facing after each.
func TestFaceEachDirection(t *testing.T) {
	e := loadFixture(t)
	p := playerAt(t, e)
	if p.X != 3 || p.Y != 6 {
		t.Fatalf("fixture start = (%d,%d), want (3,6)", p.X, p.Y)
	}

	cases := []struct {
		tx, ty uint8
		want   state.Facing
	}{
		{3, 5, state.FacingUp},
		{3, 7, state.FacingDown},
		{2, 6, state.FacingLeft},
		{4, 6, state.FacingRight},
	}
	for _, c := range cases {
		if err := skill.Face(e, c.tx, c.ty); err != nil {
			t.Fatalf("Face(%d,%d): %v", c.tx, c.ty, err)
		}
		p := playerAt(t, e)
		if p.Facing != c.want {
			t.Errorf("after Face(%d,%d) facing = %s, want %s", c.tx, c.ty, p.Facing, c.want)
		}
	}

	// Every tap was a new direction relative to the current facing (and the
	// up tile is a wall), so the player turned in place each time.
	p = playerAt(t, e)
	if p.X != 3 || p.Y != 6 {
		t.Errorf("final = (%d,%d), want unchanged (3,6)", p.X, p.Y)
	}
}

func TestFaceRejectsNonAdjacentTile(t *testing.T) {
	e := loadFixture(t)
	if err := skill.Face(e, 1, 1); err == nil {
		t.Fatal("Face(1,1) = nil, want error for a non-adjacent tile")
	}
}

// TestTalkTVSign drives the measured probe route: 2F (3,6) -> (6,1) -> push
// RIGHT over the stairs -> 1F (7,1) -> (3,2) -> face UP -> talk to the TV
// sign. No press count is asserted: the count is timing-dependent.
func TestTalkTVSign(t *testing.T) {
	e := loadFixture(t)
	p := playerAt(t, e)
	if p.MapID != 0x26 || p.X != 3 || p.Y != 6 {
		t.Fatalf("fixture start = map %02x (%d,%d), want 0x26 (3,6)", p.MapID, p.X, p.Y)
	}

	// 2F: walk to (6,1), the walkable neighbour of the (7,1) stairs.
	grid := gridFor(t, e.ROM(), 0x26)
	steps, err := world.FindPath(grid, 3, 6, 6, 1, nil)
	if err != nil {
		t.Fatalf("FindPath 2F (3,6)->(6,1): %v", err)
	}
	if err := skill.WalkPath(e, steps); err != nil {
		t.Fatalf("WalkPath 2F: %v", err)
	}
	p = playerAt(t, e)
	if p.X != 6 || p.Y != 1 {
		t.Fatalf("2F walk ended at (%d,%d), want (6,1)", p.X, p.Y)
	}

	// Push RIGHT into the stairs and wait for the map to change.
	if _, err := e.HoldUntil(emu.Right, 120, func(m *emu.Emu) bool {
		return m.Peek8(sym.CurMap) != 0x26
	}); err != nil {
		t.Fatalf("stairs: CurMap still 0x26 after 120 frames: %v", err)
	}
	p = playerAt(t, e)
	if p.MapID != 0x25 || p.X != 7 || p.Y != 1 {
		t.Fatalf("after stairs = map %02x (%d,%d), want 0x25 (7,1)", p.MapID, p.X, p.Y)
	}

	// 1F: walk to (3,2). The start tile (7,1) is solid on this map, so the
	// path only exists because of S2-1's start-tile fix.
	grid = gridFor(t, e.ROM(), 0x25)
	steps, err = world.FindPath(grid, 7, 1, 3, 2, nil)
	if err != nil {
		t.Fatalf("FindPath 1F (7,1)->(3,2): %v", err)
	}
	if err := skill.WalkPath(e, steps); err != nil {
		t.Fatalf("WalkPath 1F: %v", err)
	}
	p = playerAt(t, e)
	if p.X != 3 || p.Y != 2 {
		t.Fatalf("1F walk ended at (%d,%d), want (3,2)", p.X, p.Y)
	}

	// Face the TV sign at (3,1) and talk it to completion.
	if err := skill.Face(e, 3, 1); err != nil {
		t.Fatalf("Face(3,1): %v", err)
	}
	count, err := skill.Talk(e)
	if err != nil {
		t.Fatalf("Talk: %v", err)
	}
	if count < 1 {
		t.Errorf("Talk count = %d, want >= 1", count)
	}
	if !controllable(t, e) {
		t.Error("player is not controllable after Talk")
	}
	p = playerAt(t, e)
	if p.X != 3 || p.Y != 2 {
		t.Errorf("after Talk = (%d,%d), want unchanged (3,2)", p.X, p.Y)
	}
}
