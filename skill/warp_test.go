package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
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
	if _, err := skill.Travel(e, e.ROM(), southGate, skill.StatAwareMove(e.ROM()), 10); err != nil {
		diagFatalf(t, e, err, "Travel to the south gate: %v", err)
	}
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

// TestTravelRoute1ToPallet is the regression for the swarm's 0c->00
// failure (measured 2026-08-29, three identical runs at frame 17571):
// Route 1's south edge is tall grass at x=10 and x=11, and edgeTarget picks
// (10,35) from Place("route 1") — so the walk to the edge ends on a step
// ONTO an encounter tile. When the encounter fires after WalkPath's final
// battle check, the push used to hold its button inside a frozen battle
// and report "did not cross within 180 frames; still on map 0c at (10,35)".
// Traverse now returns ErrBattle for that; Travel fights it and re-plans
// from the same tile, where no second encounter can fire because the player
// is already standing on the grass.
//
// The tile choice was NOT the bug: x=10 is walkable on row 35 of 0x0c AND
// on row 0 of 0x00 (probed), and the push crosses in 17 frames when no
// battle fires. So the assertion is positive about arrival — wCurMap ==
// 0x00 and the player controllable — never about which column crossed.
func TestTravelRoute1ToPallet(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; run with -run '^TestTravelRoute1ToPallet$'")
	}
	m := fixture.Load(t, "pallet_town")
	if p := playerAt(t, m); p.MapID != 0x00 {
		t.Fatalf("fixture start = map %#04x, want 0x00 (Pallet Town)", p.MapID)
	}

	r1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal(`Place "route 1" not found`)
	}
	if _, err := skill.Travel(m, m.ROM(), r1, skill.StatAwareMove(m.ROM()), 20); err != nil {
		diagFatalf(t, m, err, "Travel to Route 1: %v", err)
	}
	if p := playerAt(t, m); p.MapID != 0x0c {
		t.Fatalf("after the first leg the player is on map %#04x, want 0x0c (Route 1)", p.MapID)
	}

	pal, ok := skill.Place("pallet town")
	if !ok {
		t.Fatal(`Place "pallet town" not found`)
	}
	res, err := skill.Travel(m, m.ROM(), pal, skill.StatAwareMove(m.ROM()), 20)
	if err != nil {
		diagFatalf(t, m, err, "Travel back to Pallet: %v", err)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if got := mem.U8(sym.CurMap); got != 0x00 {
		t.Fatalf("wCurMap = %#04x, want 0x00 (Pallet Town)", got)
	}
	if !state.Controllable(&mem) {
		t.Errorf("player not controllable on map %#04x after the crossing", mem.U8(sym.CurMap))
	}
	t.Logf("Route 1 -> Pallet crossed; battles fought: %d, replans: %+v", res.Battles, res.Replans)
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
