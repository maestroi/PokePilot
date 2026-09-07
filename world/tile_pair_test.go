package world

import "testing"

// gridWithPair builds a 3x1 open corridor whose three cells carry the given
// tile ids, plus one forbidden transition.
func gridWithPair(tiles []uint8, pair [2]uint8) *Grid {
	g := &Grid{
		MapID:         0x3b,
		Width:         len(tiles),
		Height:        1,
		walkable:      make([]bool, len(tiles)),
		collisionTile: append([]uint8(nil), tiles...),
		fieldTile:     make([]uint8, len(tiles)),
		tilePairs: map[[2]uint8]bool{
			{pair[0], pair[1]}: true,
			{pair[1], pair[0]}: true,
		},
	}
	for i := range g.walkable {
		g.walkable[i] = true
	}
	return g
}

// The Mt. Moon shape: both tiles walkable, the transition between them
// forbidden by the tileset's tile-pair table. Walkable must still say yes;
// Passable must say no, in both directions.
func TestPassableRejectsTilePairInBothDirections(t *testing.T) {
	g := gridWithPair([]uint8{0x20, 0x05, 0x20}, [2]uint8{0x20, 0x05})

	if !g.Walkable(0, 0) || !g.Walkable(1, 0) {
		t.Fatal("both tiles must remain walkable; the pair bans the move, not the tile")
	}
	if g.Passable(0, 0, 1, 0) {
		t.Error("0x20 -> 0x05 must be refused")
	}
	if g.Passable(1, 0, 0, 0) {
		t.Error("0x05 -> 0x20 must be refused: the game's check is symmetric")
	}
}

// A pathfinder that ignores tile pairs plans a walk the game refuses on its
// first step. With no route around, that must be reported as no path rather
// than a plan that cannot be walked.
func TestFindPathRefusesTilePairCrossing(t *testing.T) {
	g := gridWithPair([]uint8{0x20, 0x05, 0x20}, [2]uint8{0x20, 0x05})
	if _, err := FindPath(g, 0, 0, 2, 0, nil); err == nil {
		t.Fatal("FindPath crossed a tile-pair collision")
	}

	// The same corridor without the ban is walkable end to end, so the test
	// above fails for the pair and not because the corridor is broken.
	open := gridWithPair([]uint8{0x20, 0x05, 0x20}, [2]uint8{0x77, 0x88})
	steps, err := FindPath(open, 0, 0, 2, 0, nil)
	if err != nil || len(steps) != 2 {
		t.Fatalf("open corridor: steps=%v err=%v, want 2 steps and no error", steps, err)
	}
}

// The route graph's component flood must use the same rule the walker uses.
// A corridor split only by a tile-pair collision is two regions, not one: a
// flood that crosses it tells the graph a journey exists that the walker then
// refuses, which surfaces as "no route" or as a plan that dies on its first
// step.
func TestComponentsSplitOnTilePair(t *testing.T) {
	banned := gridWithPair([]uint8{0x20, 0x05, 0x20}, [2]uint8{0x20, 0x05})
	comps := components(banned)
	if comps[0][0] == comps[0][1] {
		t.Fatalf("cells either side of a tile-pair collision share component %d", comps[0][0])
	}

	open := gridWithPair([]uint8{0x20, 0x05, 0x20}, [2]uint8{0x77, 0x88})
	if c := components(open); c[0][0] != c[0][1] || c[0][1] != c[0][2] {
		t.Fatalf("open corridor split into components %v", c[0])
	}
}
