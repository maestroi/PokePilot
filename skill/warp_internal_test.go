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

		wx, wy, steps, push, err := warpTarget(h, e, g, 5, 1, nil, nil, warpTestROM(t))
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

// TestWarpTargetSkipsSolidWarpFromAdjacentTile pins the saved failure state:
// after the south-gate loop the player was controllable at (4,1), directly
// below (4,0). FindPathAdjacent can approach that tile with zero walking
// steps, but (4,0) is a wall and holding Up cannot enter it. Selection must
// therefore skip it and use the walkable (5,0) warp instead.
func TestWarpTargetSkipsSolidWarpFromAdjacentTile(t *testing.T) {
	for _, tc := range []struct {
		from, to uint8
	}{
		{0x32, 0x33},
		{0x2F, 0x0D},
	} {
		h, g := gateFixture(t, tc.from)
		e := gateEdge(t, h, tc.from, tc.to)

		wx, wy, _, _, err := warpTarget(h, e, g, 4, 1, nil, nil, warpTestROM(t))
		if err != nil {
			t.Fatalf("map %#04x: warpTarget from saved-state tile (4,1): %v", tc.from, err)
		}
		if wx != 5 || wy != 0 {
			t.Errorf("map %#04x: chosen warp tile = (%d,%d), want walkable (5,0); (4,0) is solid", tc.from, wx, wy)
		}
	}
}

// TestWarpTargetConsultsCurrentBlockers proves there is no cached selection:
// the only usable exit is (5,0), approached through (5,1). Blocking that
// approach must make the selection fail, and removing the fresh blocker must
// immediately make the same warp usable again.
func TestWarpTargetConsultsCurrentBlockers(t *testing.T) {
	for _, tc := range []struct {
		from, to uint8
	}{
		{0x32, 0x33},
		{0x2F, 0x0D},
	} {
		h, g := gateFixture(t, tc.from)
		e := gateEdge(t, h, tc.from, tc.to)

		blocked := map[[2]int]bool{{5, 1}: true}
		if _, _, _, _, err := warpTarget(h, e, g, 4, 2, blocked, nil, warpTestROM(t)); err == nil {
			t.Errorf("map %#04x: selection succeeded with the only approach (5,1) blocked", tc.from)
		}
		got, err := selectWarp(t, h, e, g, 4, 2)
		if err != nil {
			t.Fatalf("map %#04x: selection after blocker moved: %v", tc.from, err)
		}
		if got != [2]int{5, 0} {
			t.Errorf("map %#04x: after blocker moved chose %v, want walkable (5,0)", tc.from, got)
		}
	}
}

// TestWarpTargetExcludeFallsBackToOtherCandidate pins
// run-13kws9zfzq7ka1p4bmd16c9yg3: Route 7 Gate's Saffron-side door is a
// genuine ROM pair, (18,9) and (18,10), both landing on adjacent tiles of
// ROUTE_7_GATE. Measured with the emulator, no orthogonal approach to (18,9)
// ever fires the door (its tile is not a recognized door/warp graphic, and
// the "two tiles ahead" collision check never matches either), while pushing
// west from (18,10) does. Traverse bans an exhausted candidate and asks
// warpTarget again; this proves that ask actually returns the other tile
// instead of the same unreachable one.
func TestWarpTargetExcludeFallsBackToOtherCandidate(t *testing.T) {
	const route7 = 0x12
	const route7Gate = 0x4c
	h, g := gateFixture(t, route7)
	romData := warpTestROM(t)
	e := world.Edge{Kind: world.EdgeWarp, From: route7, To: route7Gate, WarpX: 18, WarpY: 9}

	wx, wy, _, _, err := warpTarget(h, e, g, 19, 9, nil, nil, romData)
	if err != nil {
		t.Fatalf("warpTarget (no exclusion): %v", err)
	}
	if wx != 18 || wy != 9 {
		t.Fatalf("warpTarget (no exclusion) chose (%d,%d), want the ROM-first candidate (18,9)", wx, wy)
	}

	excluded := map[[2]int]bool{{18, 9}: true}
	wx, wy, _, _, err = warpTarget(h, e, g, 19, 9, nil, excluded, romData)
	if err != nil {
		t.Fatalf("warpTarget (18,9 excluded): %v", err)
	}
	if wx != 18 || wy != 10 {
		t.Errorf("warpTarget (18,9 excluded) chose (%d,%d), want the other ROM candidate (18,10)", wx, wy)
	}
}

// TestWarpAvoidanceBansEveryOtherWarpTile pins the fix for
// run-3cefsxn84apv3126k7vkfk517y round 5: a walk toward an ordinary
// connection edge (Cerulean's east border, band 16..17) is free to path
// across any other warp tile the collision grid says is walkable, since
// world.FindPath knows nothing about warps. Cerulean City exposes the Badge
// House's front door at (9,11); stepping on it fires the warp, and the
// Badge House's OTHER exterior door, (9,9), is a genuine dead pocket
// (CeruleanBadgeHouse's own warp table sends it there, and the collision
// grid shows it walled off from the rest of the city on every side but the
// warp itself — measured with the probe, PROBE_MAP=0x03 PROBE_AT=9,9,
// north/south/east/west edges all unreachable). Once diverted there, no
// re-plan can walk out again without re-crossing the same house. warpTarget
// has always banned every other warp tile from a warp APPROACH
// (approachBlocked); warpAvoidance is that same rule shared with ordinary
// connection walks.
func TestWarpAvoidanceBansEveryOtherWarpTile(t *testing.T) {
	const ceruleanCity = 0x03
	romData := warpTestROM(t)
	h, err := rom.ParseMap(romData, ceruleanCity)
	if err != nil {
		t.Fatalf("ParseMap CeruleanCity: %v", err)
	}
	if len(h.Warps) < 2 {
		t.Fatalf("CeruleanCity has %d warp(s), want at least 2 to test avoidance", len(h.Warps))
	}

	blocked := warpAvoidance(h, 5, 5, nil)
	for _, w := range h.Warps {
		if !blocked[[2]int{int(w.X), int(w.Y)}] {
			t.Errorf("warpAvoidance from (5,5) left warp tile (%d,%d) unblocked", w.X, w.Y)
		}
	}

	// The Badge House's front door, (9,11), must be one of the banned tiles
	// standing anywhere else — it is not itself the player's position here.
	if !blocked[[2]int{9, 11}] {
		t.Errorf("warpAvoidance did not ban the Badge House door (9,11)")
	}

	// Standing exactly on a warp tile must exempt that one tile: arriving on
	// it does not refire it, and a caller there must still be free to walk
	// off in any direction.
	standingOnDoor := warpAvoidance(h, 9, 11, nil)
	if standingOnDoor[[2]int{9, 11}] {
		t.Errorf("warpAvoidance blocked the tile the caller is standing on, (9,11)")
	}
}

// selectWarp runs one warpTarget selection and returns the chosen tile.
func selectWarp(t *testing.T, h rom.MapHeader, e world.Edge, g *world.Grid, sx, sy int) ([2]int, error) {
	t.Helper()
	wx, wy, _, _, err := warpTarget(h, e, g, sx, sy, nil, nil, warpTestROM(t))
	if err != nil {
		return [2]int{}, err
	}
	return [2]int{wx, wy}, nil
}

func warpTestROM(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("POKEMON_RED_ROM"))
	if err != nil {
		t.Skipf("POKEMON_RED_ROM not set: %v", err)
	}
	return data
}


func TestMergeBlockedTilesAddsExtraWithoutMutatingBase(t *testing.T) {
	base := map[[2]int]bool{{1, 2}: true}
	extra := map[[2]int]bool{{3, 4}: true}
	merged := mergeBlockedTiles(base, extra)

	if !merged[[2]int{1, 2}] || !merged[[2]int{3, 4}] {
		t.Fatalf("merged blockers = %v, want base and extra tiles", merged)
	}
	if base[[2]int{3, 4}] {
		t.Fatalf("mergeBlockedTiles mutated base blockers: %v", base)
	}
}
