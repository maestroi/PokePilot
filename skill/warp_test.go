package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
)

// TestTraverseGateWarp is the ROM-backed end-to-end proof of the warp-tile
// selection: the south gate's warp table offers (4,0) first, and (4,0) is
// non-walkable in this ROM with the pathfinder's only route to it crossing
// the (5,0) warp. Handing Traverse the (4,0) edge must still cross the gate,
// via the reachable (5,0) tile. The landing position pins which warp fired:
// (5,0) is warp id 4 on the gate and lands at forest (17,47); (4,0) is id 3
// and would land at (16,47).
func TestTraverseGateWarp(t *testing.T) {
	e := fixture.Load(t, "post_errand")
	p := playerAt(t, e)
	if p.MapID != 0x01 {
		t.Fatalf("fixture start = map %02x, want 0x01 (Viridian City)", p.MapID)
	}

	// Walk to the gate corridor, one row below the warp line — the same
	// position the old crossGate helper started from.
	southGate := skill.Destination{Map: 0x32, X: 5, Y: 1}
	travelFightsThrough(t, e, e.ROM(), southGate, skill.StatAwareMove(e.ROM()), 10)
	p = playerAt(t, e)
	if p.MapID != 0x32 || p.X != 5 || p.Y != 1 {
		t.Fatalf("travel stopped at map %02x (%d,%d), want the south gate 0x32 (5,1)", p.MapID, p.X, p.Y)
	}

	g := graphFor(t, e.ROM())
	edges := edgesFromTo(g, 0x32, 0x33)
	if len(edges) != 2 {
		t.Fatalf("edges 0x32->0x33 = %d, want exactly 2 (the two gate warp tiles)", len(edges))
	}
	var bad world.Edge
	for _, c := range edges {
		if c.WarpX == 4 && c.WarpY == 0 {
			bad = c
			break
		}
	}
	if bad.To != 0x33 {
		t.Fatalf("no (4,0) warp edge 0x32->0x33 in the graph: %+v", edges)
	}

	// The (4,0) edge is the one the old Traverse died on: the player cannot
	// stand on (4,0) and the pathfinder's route to it fires the (5,0) warp
	// mid-walk. Traverse must pick the reachable tile itself.
	if err := skill.Traverse(e, e.ROM(), bad); err != nil {
		t.Fatalf("Traverse 0x32->0x33 on the (4,0) edge: %v", err)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	p = state.DecodePlayer(&mem)
	if p.MapID != 0x33 {
		t.Fatalf("CurMap = %02x, want 0x33 (Viridian Forest)", p.MapID)
	}
	if p.X != 17 || p.Y != 47 {
		t.Errorf("player = (%d,%d), want (17,47): the (5,0) warp's landing; (16,47) would mean the (4,0) warp fired", p.X, p.Y)
	}
	if !state.Controllable(&mem) {
		t.Error("player not controllable after the gate crossing")
	}
	t.Logf("crossed the south gate via the reachable warp tile, landed at (%d,%d)", p.X, p.Y)
}

// graphFor builds the map-level graph from the live ROM.
func graphFor(t *testing.T, romData []byte) *world.Graph {
	t.Helper()
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	return g
}

// edgesFromTo returns the graph edges from one map to another.
func edgesFromTo(g *world.Graph, from, to uint8) []world.Edge {
	var out []world.Edge
	for _, e := range g.Edges[from] {
		if e.To == to {
			out = append(out, e)
		}
	}
	return out
}

// TestTraverseWarpChain takes the single 0x26 -> 0x25 warp (2F stairs down
// to 1F), then a 0x25 -> 0x00 warp (1F door to Pallet Town), on one
// emulator. The second leg starts on a tile (7,1) that the destination map
// calls solid, so it also covers the pathfinder's solid-start handling.
func TestTraverseWarpChain(t *testing.T) {
	e := loadFixture(t)
	p := playerAt(t, e)
	if p.MapID != 0x26 || p.X != 3 || p.Y != 6 {
		t.Fatalf("fixture start = map %02x (%d,%d), want 0x26 (3,6)", p.MapID, p.X, p.Y)
	}

	g := graphFor(t, e.ROM())
	edges := edgesFromTo(g, 0x26, 0x25)
	if len(edges) != 1 {
		t.Fatalf("edges 0x26->0x25 = %d, want exactly 1", len(edges))
	}

	if err := skill.Traverse(e, e.ROM(), edges[0]); err != nil {
		t.Fatalf("Traverse 0x26->0x25: %v", err)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	p = state.DecodePlayer(&mem)
	w := state.DecodeWorld(&mem)
	if p.MapID != 0x25 {
		t.Fatalf("CurMap = %02x, want 0x25", p.MapID)
	}
	if p.X != 7 || p.Y != 1 {
		t.Errorf("player = (%d,%d), want (7,1)", p.X, p.Y)
	}
	if w.Width == 0 || w.Height == 0 {
		t.Errorf("map dimensions = %dx%d, want non-zero", w.Width, w.Height)
	}
	if !state.Controllable(&mem) {
		t.Error("player not controllable after 0x26->0x25")
	}

	// The 1F door is two tiles wide, so the graph carries two warp edges to
	// Pallet Town; pick one deterministically (lowest y, then lowest x).
	edges = edgesFromTo(g, 0x25, 0x00)
	if len(edges) < 1 {
		t.Fatalf("edges 0x25->0x00 = 0, want at least 1")
	}
	edge := edges[0]
	for _, c := range edges[1:] {
		if c.WarpY < edge.WarpY || (c.WarpY == edge.WarpY && c.WarpX < edge.WarpX) {
			edge = c
		}
	}
	if err := skill.Traverse(e, e.ROM(), edge); err != nil {
		t.Fatalf("Traverse 0x25->0x00: %v", err)
	}

	state.Snapshot(e, &mem)
	p = state.DecodePlayer(&mem)
	if p.MapID != 0x00 {
		t.Fatalf("CurMap = %02x, want 0x00 (Pallet Town)", p.MapID)
	}
	if !state.Controllable(&mem) {
		t.Error("player not controllable after 0x25->0x00")
	}
}
