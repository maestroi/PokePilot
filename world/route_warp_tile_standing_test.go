package world

import (
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

// TestFindRouteAtIgnoresUnreachableWarpWhenStandingOnAWarpTile is the Saffron
// Gym shape (run-opn83x1wj99vp9241oyhcacb): the player can be standing
// exactly on a warp tile itself — a gym's exit door, a checkpoint resumed
// mid-warp — and componentsWithBlocked deliberately excludes every warp tile
// from the walkable-component flood so a teleporter can't falsely bridge the
// rooms it connects. Before the fix, standingComponentAt returned nil for
// that tile, canExit read nil as "component unknown, don't filter," and the
// router treated every same-map warp edge as reachable from here — including
// one whose pad sits behind a wall in a room this position cannot walk to.
// FindRouteAt must reject that edge and take the one whose pad is actually in
// the same walkable room, even when the unreachable edge is examined first.
func TestFindRouteAtIgnoresUnreachableWarpWhenStandingOnAWarpTile(t *testing.T) {
	// Map 1, one row: [0]=door (warp, player stands here) [1]=room A
	// [2]=near pad (warp, in room A) [3]=wall [4]=far pad (warp, in room B)
	// [5]=room B. Room A and room B are not connected by walking.
	const (
		door    = 0
		roomA   = 1
		nearPad = 2
		wall    = 3
		farPad  = 4
		roomB   = 5
	)
	farEdge := Edge{Kind: EdgeWarp, From: 1, To: 9, WarpX: farPad, WarpY: 0}
	nearEdge := Edge{Kind: EdgeWarp, From: 1, To: 9, WarpX: nearPad, WarpY: 0}
	g := &Graph{
		// farEdge listed first: BFS explores edges in slice order, so a
		// route that picks it over nearEdge proves the bug (first hop
		// unconstrained), not slice-order luck.
		Edges: map[uint8][]Edge{
			1: {farEdge, nearEdge},
			9: nil,
		},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{0, 1, 0, 0, 0, 2}},
			9: {{1}},
		},
		warps: map[uint8][]worldmodel.Warp{
			1: {{X: door, Y: 0}, {X: nearPad, Y: 0}, {X: farPad, Y: 0}},
		},
		exitComps: map[Edge][]int{
			farEdge:  {2}, // farPad's only walkable neighbor is room B
			nearEdge: {1}, // nearPad's only walkable neighbor is room A
		},
		entryComps: map[Edge][]int{
			farEdge:  {1},
			nearEdge: {1},
		},
	}

	route, err := FindRouteAt(g, 1, 9, door, 0, nil)
	if err != nil {
		t.Fatalf("FindRouteAt: %v", err)
	}
	if len(route) != 1 || route[0] != nearEdge {
		t.Fatalf("route = %v, want [nearEdge] (%+v); the unreachable far pad must not be offered", route, nearEdge)
	}
}

// TestStandingComponentAtDistinguishesWarpTileFromUnwalkablePadding guards
// the neighbor-fallback against over-applying: a plain unwalkable tile (a
// wall, padding on a connection border) must stay "unknown" rather than
// borrowing a neighbor's component, or a padding tile would look like part
// of the walkable room beside it (this broke connection band splitting when
// tried without the isWarpTile guard).
func TestStandingComponentAtDistinguishesWarpTileFromUnwalkablePadding(t *testing.T) {
	g := &Graph{
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1, 0, 0}},
		},
		warps: map[uint8][]worldmodel.Warp{
			1: {{X: 1, Y: 0}}, // only x=1 is a warp tile; x=2 is plain wall/padding
		},
	}

	if got := standingComponentAt(g, 1, 1, 0); len(got) != 1 || got[0] != 1 {
		t.Fatalf("standingComponentAt on warp tile = %v, want [1] (neighbor room)", got)
	}
	if got := standingComponentAt(g, 1, 2, 0); got != nil {
		t.Fatalf("standingComponentAt on plain unwalkable tile = %v, want nil (stay unknown)", got)
	}
}
