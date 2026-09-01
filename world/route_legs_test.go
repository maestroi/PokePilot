package world

import (
	"testing"
)

// These tests cover the two phantom legs S10-11 named: leg 5 (Route 2 ->
// Viridian across a solid row) and leg 7 (Route 22 -> Route 23 through the
// gate 0xC1 whose warps all point at 0xFF). They are graph-level: the question
// is purely about routing, so no emulator is involved.

// TestBuildGraphGateResolution checks that the gate 0xC1's 0xFF warps resolve
// to the route on the side each door faces: the north doors to Route 23, the
// south doors to Route 22.
func TestBuildGraphGateResolution(t *testing.T) {
	g := loadGraph(t)
	for _, w := range [][2]uint8{{4, 0}, {5, 0}} {
		if !hasWarpEdge(g, 0xC1, 0x22, w[0], w[1]) {
			t.Errorf("gate north door (%d,%d) does not resolve to Route 23 0x22", w[0], w[1])
		}
	}
	for _, w := range [][2]uint8{{4, 7}, {5, 7}} {
		if !hasWarpEdge(g, 0xC1, 0x21, w[0], w[1]) {
			t.Errorf("gate south door (%d,%d) does not resolve to Route 22 0x21", w[0], w[1])
		}
	}
}

// TestRouteLeg7Gate checks that a query from Route 22 to Route 23 returns a
// route through the gate 0xC1, not the phantom direct N-edge connection.
func TestRouteLeg7Gate(t *testing.T) {
	g := loadGraph(t)
	route, err := FindRoute(g, 0x21, 0x22)
	if err != nil {
		t.Fatalf("FindRoute(0x21, 0x22): %v", err)
	}
	if len(route) == 1 {
		t.Fatalf("route is the phantom direct connection: %+v", route)
	}
	if !routeVisits(route, 0xC1) {
		t.Fatalf("route does not pass through gate 0xC1: %+v", route)
	}
}

// TestRouteLeg7RoadDetours is the component check in action. The gate door
// (8,5) on Route 22 sits in a sealed component: under every collision sub-tile
// rule it is either solid or cut off from the road the player walks. So a
// query rooted at the road (where the player enters Route 22) must NOT take
// the gate leg — it detours the long way around — even though the
// graph-level FindRoute (no position) offers the gate. This is the tile-level
// truth S10-11 flagged for the emulator; named here so the graph's optimism is
// on the record.
func TestRouteLeg7RoadDetours(t *testing.T) {
	g := loadGraph(t)
	x, y, ok := g.walkableEdgeTile(0x21, dirEast)
	if !ok {
		t.Fatal("no walkable east-edge tile on Route 22")
	}
	route, err := FindRouteAt(g, 0x21, 0x22, x, y, nil)
	if err != nil {
		t.Fatalf("FindRouteAt(0x21, 0x22) from the road: %v", err)
	}
	if routeVisits(route, 0xC1) {
		t.Fatalf("route from the road takes the sealed gate door: %+v", route)
	}
}

// TestRouteNegativeUnreachable is the negative case: a map that is dead data
// (no edges in or out) is genuinely not walkable, and the search must report
// no route rather than invent one. The component-aware leg predicate only ever
// rejects edges, so it cannot manufacture a path to a node it cannot reach.
func TestRouteNegativeUnreachable(t *testing.T) {
	g := loadGraph(t)
	if _, err := FindRoute(g, 0x00, 0xF0); err != ErrNoRoute {
		t.Fatalf("FindRoute(0x00, 0xF0) = %v, want ErrNoRoute (0xF0 is dead data)", err)
	}
}

// TestRouteLeg5PewterSideUsesForest checks that a query from Route 2's
// Pewter-side entry (the north band) to Viridian goes through the forest
// gates, not across the solid row 22.
func TestRouteLeg5PewterSideUsesForest(t *testing.T) {
	g := loadGraph(t)
	x, y, ok := g.walkableEdgeTile(0x0D, dirNorth)
	if !ok {
		t.Fatal("no walkable north-edge tile on Route 2")
	}
	route, err := FindRouteAt(g, 0x0D, 0x01, x, y, nil)
	if err != nil {
		t.Fatalf("FindRouteAt(0x0D, 0x01) from the north band: %v", err)
	}
	if len(route) == 1 {
		t.Fatalf("route is the phantom direct connection: %+v", route)
	}
	if !routeVisits(route, 0x33) {
		t.Fatalf("route does not pass through Viridian Forest 0x33: %+v", route)
	}
}

// TestRouteLeg5DirectConnectionIsPhantom checks the other side of the same
// ledge: from the south band the direct S-edge connection to Viridian is the
// route, so the leg is not unconditionally rejected — only when the entry
// component cannot reach it.
func TestRouteLeg5DirectConnectionIsPhantom(t *testing.T) {
	g := loadGraph(t)
	x, y, ok := g.walkableEdgeTile(0x0D, dirSouth)
	if !ok {
		t.Fatal("no walkable south-edge tile on Route 2")
	}
	route, err := FindRouteAt(g, 0x0D, 0x01, x, y, nil)
	if err != nil {
		t.Fatalf("FindRouteAt(0x0D, 0x01) from the south band: %v", err)
	}
	if len(route) != 1 || route[0].From != 0x0D || route[0].To != 0x01 {
		t.Fatalf("route from the south band = %+v, want the direct S-edge", route)
	}
}

// TestRouteRegressionPalletPewterCerulean checks that pre-existing routing is
// unchanged: Pallet -> Pewter and Pewter -> Cerulean still resolve.
func TestRouteRegressionPalletPewterCerulean(t *testing.T) {
	g := loadGraph(t)
	if _, err := FindRoute(g, 0x00, 0x02); err != nil {
		t.Fatalf("FindRoute(0x00, 0x02) Pallet->Pewter: %v", err)
	}
	if _, err := FindRoute(g, 0x02, 0x03); err != nil {
		t.Fatalf("FindRoute(0x02, 0x03) Pewter->Cerulean: %v", err)
	}
}

// walkableEdgeTile returns a walkable tile on map m's edge in direction dir,
// or ok=false if that edge has none. Test-only accessor for the component
// labels BuildGraph computes.
func (g *Graph) walkableEdgeTile(m, dir uint8) (int, int, bool) {
	comps := g.comps[m]
	if comps == nil {
		return 0, 0, false
	}
	w, h := g.tiles[m].w, g.tiles[m].h
	walk := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < w && y < h && comps[y][x] != 0
	}
	switch dir {
	case dirNorth:
		for x := 0; x < w; x++ {
			if walk(x, 0) {
				return x, 0, true
			}
		}
	case dirSouth:
		for x := 0; x < w; x++ {
			if walk(x, h-1) {
				return x, h - 1, true
			}
		}
	case dirEast:
		for y := 0; y < h; y++ {
			if walk(w-1, y) {
				return w - 1, y, true
			}
		}
	case dirWest:
		for y := 0; y < h; y++ {
			if walk(0, y) {
				return 0, y, true
			}
		}
	}
	return 0, 0, false
}

// routeVisits reports whether the route enters or leaves map m.
func routeVisits(route []Edge, m uint8) bool {
	for _, e := range route {
		if e.From == m || e.To == m {
			return true
		}
	}
	return false
}
