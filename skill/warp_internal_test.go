package skill

// Internal tests for warpTarget, the warp-tile selection inside Traverse.
// They live in package skill because warpTarget is unexported; the ROM-backed
// end-to-end coverage of the exported Traverse itself is in warp_test.go.
//
// Both forest gate maps expose two warp tiles to the same destination, and
// the first one in the warp table, (4,0), is non-walkable in this ROM:
//
//	ViridianForestSouthGate: warp_event 4,0 -> VIRIDIAN_FOREST ; warp_event 5,0 -> VIRIDIAN_FOREST
//	ViridianForestNorthGate: warp_event 4,0 -> LAST_MAP        ; warp_event 5,0 -> LAST_MAP
//
// Measured with the probe (POKEMON_RED_ROM set, PROBE_AT=5,1), both maps are
// 10x8 with row 0 reading "#####.####": (4,0) is a wall, (5,0) is the only
// walkable exit, and the corridor runs down x 4-5. From the corridor tile
// (5,1) the only route the pathfinder offers to (4,0) runs through the
// (5,0) warp tile itself, which fires that warp mid-walk — so (5,0) is the
// warp a standing player can actually cross.

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// gateFixture loads the header and collision grid of one gate map from the
// live ROM, skipping when the ROM is not available (the same env gate as
// TestProbe).
func gateFixture(t *testing.T, id uint8) (rom.MapHeader, *world.Grid) {
	t.Helper()
	romData, err := os.ReadFile(os.Getenv("POKEMON_RED_ROM"))
	if err != nil {
		t.Skipf("POKEMON_RED_ROM not set: %v", err)
	}
	h, err := rom.ParseMap(romData, id)
	if err != nil {
		t.Fatalf("ParseMap %#04x: %v", id, err)
	}
	g, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build %#04x: %v", id, err)
	}
	return h, g
}

// gateEdge builds the graph edge the planner offers for a gate crossing: the
// first warp tile in the table, (4,0) — the one the old Traverse walked to
// and died on.
func gateEdge(t *testing.T, h rom.MapHeader, from, to uint8) world.Edge {
	t.Helper()
	for _, w := range h.Warps {
		if int(w.X) == 4 && int(w.Y) == 0 {
			return world.Edge{Kind: world.EdgeWarp, From: from, To: to, WarpX: w.X, WarpY: w.Y}
		}
	}
	t.Fatalf("map %#04x has no warp at (4,0)", from)
	return world.Edge{}
}

// TestWarpTargetTargetsReachableTile proves the new selection on BOTH gates:
// standing in the corridor at (5,1), the chosen warp tile is the reachable
// (5,0), not the unreachable (4,0). The assertion is on the chosen tile
// itself, not on the existence of a path.
func TestWarpTargetTargetsReachableTile(t *testing.T) {
	for _, tc := range []struct {
		from, to uint8
	}{
		{0x32, 0x33}, // south gate -> Viridian Forest
		{0x2F, 0x0D}, // north gate -> Route 2 (LAST_MAP resolved)
	} {
		h, g := gateFixture(t, tc.from)
		e := gateEdge(t, h, tc.from, tc.to)

		wx, wy, steps, push, err := warpTarget(h, e, g, 5, 1, nil)
		if err != nil {
			t.Fatalf("map %#04x: warpTarget from (5,1): %v", tc.from, err)
		}
		if wx != 5 || wy != 0 {
			t.Errorf("map %#04x: chosen warp tile = (%d,%d), want the reachable (5,0), not the unreachable (4,0); steps=%v push=%v",
				tc.from, wx, wy, steps, push)
		}

		// The chosen tile must lead to the destination: the walk ends on a
		// walkable tile orthogonally adjacent to (5,0) and the push enters
		// it.
		if !g.InBounds(wx, wy) || !g.Walkable(wx, wy) {
			t.Errorf("map %#04x: chosen tile (%d,%d) is not a walkable warp tile", tc.from, wx, wy)
		}
	}
}

// TestWarpTargetConsultsPathfinderPerPosition proves there is no cache: the
// selection is a fact about where the player stands RIGHT NOW. Two
// selections from different positions each consult the pathfinder afresh and
// come out different — (4,0) is unreachable from (5,1) because the only
// route to it crosses the (5,0) warp, but from (4,2) the corridor tile (4,1)
// is a clean approach, so (4,0) IS the chosen tile there. A selection cached
// as a property of the map or the warp (the 871f9d4 / 56a1b22 mistake) would
// return the same tile for both positions.
func TestWarpTargetConsultsPathfinderPerPosition(t *testing.T) {
	for _, tc := range []struct {
		from, to uint8
	}{
		{0x32, 0x33},
		{0x2F, 0x0D},
	} {
		h, g := gateFixture(t, tc.from)
		e := gateEdge(t, h, tc.from, tc.to)

		// Same map, same edge, three player positions: the result must
		// follow the position, not the map, and be stable across calls so
		// that no earlier selection leaks into a later one.
		first, err := selectWarp(t, h, e, g, 5, 1)
		if err != nil {
			t.Fatalf("map %#04x: selection from (5,1): %v", tc.from, err)
		}
		second, err := selectWarp(t, h, e, g, 4, 2)
		if err != nil {
			t.Fatalf("map %#04x: selection from (4,2): %v", tc.from, err)
		}
		third, err := selectWarp(t, h, e, g, 5, 1)
		if err != nil {
			t.Fatalf("map %#04x: selection from (5,1) again: %v", tc.from, err)
		}

		if first != [2]int{5, 0} {
			t.Errorf("map %#04x: from (5,1) chose %v, want (5,0): (4,0)'s only route crosses the (5,0) warp", tc.from, first)
		}
		if second != [2]int{4, 0} {
			t.Errorf("map %#04x: from (4,2) chose %v, want (4,0): (4,1) is a clean approach, so the pathfinder reaches it afresh", tc.from, second)
		}
		if third != first {
			t.Errorf("map %#04x: from (5,1) chose %v then %v: the selection depends on call history, not just position", tc.from, first, third)
		}
	}
}

// selectWarp runs one warpTarget selection and returns the chosen tile.
func selectWarp(t *testing.T, h rom.MapHeader, e world.Edge, g *world.Grid, sx, sy int) ([2]int, error) {
	t.Helper()
	wx, wy, _, _, err := warpTarget(h, e, g, sx, sy, nil)
	if err != nil {
		return [2]int{}, err
	}
	return [2]int{wx, wy}, nil
}
