package world

import (
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

// TestFindRouteAtIgnoresUnreachableWarpWhenStandingOnAWarpTile is the Saffron
// Gym shape measured on:
//
//   - run-opn83x1wj99vp9241oyhcacb (fixed in #1414)
//   - run-3ryd6j6etrlvo3eof6m6vuiu6l / triage:978e898d718fbf45 / farm-issue #1412
//   - run-1uyafua3jt0o9egm3y6lg9np3 / triage:6ffbb6bf79d245b0 / farm-issue #1413
//     (same symptom on runner 9ec6751c: go_to blocked route_replan_exhausted
//     saffron gym — 8 re-plans from map b2 at (8,17) toward (9,9), last leg
//     traverseIntraMapWarp with no reachable source pad for warp (5,9))
//
// The player can be standing exactly on a warp tile itself — a gym's exit
// door, a checkpoint resumed mid-warp — and componentsWithBlocked deliberately
// excludes every warp tile from the walkable-component flood so a teleporter
// can't falsely bridge the rooms it connects. Before the fix,
// standingComponentAt returned nil for that tile, canExit read nil as
// "component unknown, don't filter," and the router treated every same-map
// warp edge as reachable from here — including one whose pad sits behind a
// wall in a room this position cannot walk to. FindRouteAt must reject that
// edge and take the one whose pad is actually in the same walkable room, even
// when the unreachable edge is examined first.
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

// TestFindRouteAtDestinationIgnoresUnreachableSameMapWarpWhenStandingOnWarpTile
// is the exact go_to saffron gym farm shape: same-map destination tile, player
// standing on the gym exit warp, unreachable same-map teleporter listed first.
// Sibling fingerprints on runner 9ec6751c before #1414 deployed:
//
//   - run-3ryd6j6etrlvo3eof6m6vuiu6l / triage:978e898d718fbf45 / farm-issue #1412
//   - run-1uyafua3jt0o9egm3y6lg9np3 / triage:6ffbb6bf79d245b0 / farm-issue #1413
//
// Both exhausted re-plans offering warp (5,9) from SAFFRON_GYM (8,17) toward
// Place (9,9). See also TestSaffronGymExitDoorRoutesViaReachableTeleporter.
func TestFindRouteAtDestinationIgnoresUnreachableSameMapWarpWhenStandingOnWarpTile(t *testing.T) {
	const (
		door    = 0 // player stands here (exit warp)
		nearPad = 2 // reachable teleporter in room A → room B
		farPad  = 4 // unreachable teleporter already in room B
		roomB   = 5
	)
	farEdge := Edge{Kind: EdgeWarp, From: 1, To: 1, WarpX: farPad, WarpY: 0}
	nearEdge := Edge{Kind: EdgeWarp, From: 1, To: 1, WarpX: nearPad, WarpY: 0}
	g := &Graph{
		// farEdge first: unconstrained first-hop search would pick it.
		Edges: map[uint8][]Edge{
			1: {farEdge, nearEdge},
		},
		componentAware: true,
		comps: map[uint8][][]int{
			// door excluded (0); roomA=1 around nearPad; roomB=2 around farPad/dest
			1: {{0, 1, 0, 0, 0, 2}},
		},
		warps: map[uint8][]worldmodel.Warp{
			1: {{X: door, Y: 0}, {X: nearPad, Y: 0}, {X: farPad, Y: 0}},
		},
		exitComps: map[Edge][]int{
			farEdge:  {2}, // only usable once already in room B
			nearEdge: {1}, // usable from room A (door's walkable neighbor)
		},
		entryComps: map[Edge][]int{
			farEdge:  {2},
			nearEdge: {2}, // near pad teleports into room B
		},
	}

	route, err := FindRouteAtDestination(g, 1, 1, door, 0, roomB, 0, nil)
	if err != nil {
		t.Fatalf("FindRouteAtDestination to room B: %v", err)
	}
	if len(route) != 1 || route[0] != nearEdge {
		t.Fatalf("route = %v, want [nearEdge]; unreachable far pad must not be offered from the exit door", route)
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
