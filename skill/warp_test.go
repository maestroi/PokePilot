package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

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
