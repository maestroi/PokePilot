package world

import (
	"errors"
	"testing"
)

func TestFindRouteSameMap(t *testing.T) {
	g := loadGraph(t)
	route, err := FindRoute(g, 0x26, 0x26)
	if err != nil {
		t.Fatalf("FindRoute(0x26, 0x26): %v", err)
	}
	if len(route) != 0 {
		t.Errorf("route length = %d, want 0", len(route))
	}
}

func TestFindRouteDirectWarp(t *testing.T) {
	g := loadGraph(t)
	route, err := FindRoute(g, 0x26, 0x25)
	if err != nil {
		t.Fatalf("FindRoute(0x26, 0x25): %v", err)
	}
	if len(route) != 1 {
		t.Fatalf("route length = %d, want 1: %+v", len(route), route)
	}
	e := route[0]
	if e.Kind != EdgeWarp || e.From != 0x26 || e.To != 0x25 || e.WarpX != 7 || e.WarpY != 1 {
		t.Errorf("edge = %+v, want EdgeWarp 0x26->0x25 at (7,1)", e)
	}
}

func TestFindRouteBedroomToPokeCenter(t *testing.T) {
	g := loadGraph(t)
	route, err := FindRoute(g, 0x26, 0x29)
	if err != nil {
		t.Fatalf("FindRoute(0x26, 0x29): %v", err)
	}
	if len(route) == 0 {
		t.Fatal("route is empty, want a non-empty route")
	}
	// Contiguity: first edge leaves 0x26, each edge continues from the
	// previous edge's To, last edge arrives at 0x29.
	for i, e := range route {
		wantFrom := uint8(0x26)
		if i > 0 {
			wantFrom = route[i-1].To
		}
		if e.From != wantFrom {
			t.Fatalf("route not contiguous at %d: edge %+v From=%02X want %02X", i, e, e.From, wantFrom)
		}
	}
	if last := route[len(route)-1].To; last != 0x29 {
		t.Errorf("last edge To = %02X, want 0x29", last)
	}
	// The route must pass through Route 1 (0x0C) via a connection.
	throughRoute1 := false
	for _, e := range route {
		if e.Kind == EdgeConnection && (e.From == 0x0C || e.To == 0x0C) {
			throughRoute1 = true
			break
		}
	}
	if !throughRoute1 {
		t.Errorf("route does not pass through Route 1 (0x0C) via an EdgeConnection: %+v", route)
	}
}

func TestFindRouteNoRoute(t *testing.T) {
	g := loadGraph(t)
	// Find a map id that no edge reaches; it is unreachable from anywhere.
	reached := make(map[uint8]bool)
	for _, edges := range g.Edges {
		for _, e := range edges {
			reached[e.To] = true
		}
	}
	target, found := uint8(0), false
	for id := uint8(0); id <= maxMapID; id++ {
		if !reached[id] {
			target, found = id, true
			break
		}
	}
	if !found {
		t.Skip("no unreachable map id found in graph")
	}
	if _, err := FindRoute(g, 0x26, target); err != ErrNoRoute {
		t.Errorf("FindRoute(0x26, %02X) err = %v, want ErrNoRoute", target, err)
	}
}

// TestFindRouteAvoiding: the map graph knows which maps touch, not which
// are walkable between. When the leg the caller is standing in front of
// turns out to be unwalkable it reports that and asks again, and the
// search must find the way round rather than insisting on the short
// impossible one.
func TestFindRouteAvoiding(t *testing.T) {
	// 1 --conn--> 3        (short, and unwalkable from where we stand)
	// 1 --warp--> 2 --> 3  (longer, and real)
	short := Edge{Kind: EdgeConnection, From: 1, To: 3}
	viaWarp := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 5, WarpY: 5}
	g := &Graph{Edges: map[uint8][]Edge{
		1: {short, viaWarp},
		2: {{Kind: EdgeWarp, From: 2, To: 3, WarpX: 1, WarpY: 1}},
		3: nil,
	}}

	route, err := FindRoute(g, 1, 3)
	if err != nil {
		t.Fatalf("FindRoute: %v", err)
	}
	if len(route) != 1 || route[0] != short {
		t.Fatalf("unblocked route = %v, want the short 1->3 leg", route)
	}

	route, err = FindRouteAvoiding(g, 1, 3, map[Edge]bool{short: true})
	if err != nil {
		t.Fatalf("FindRouteAvoiding: %v", err)
	}
	if len(route) != 2 || route[0] != viaWarp {
		t.Fatalf("route avoiding the blocked leg = %v, want the 2-hop way round", route)
	}

	// Barring every first hop leaves no route, and that is an error
	// rather than a route that cannot be walked.
	_, err = FindRouteAvoiding(g, 1, 3, map[Edge]bool{short: true, viaWarp: true})
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute when every first hop is barred", err)
	}
}

// TestFindRouteAvoidingReEntersTheStartMap is the Route 2 shape, and the
// reason this search is keyed by edge and bars only the first hop.
//
// A ledge splits Route 2 (0x0D) across its width. Its northern connection
// to Pewter (0x02) is unwalkable from the southern landing tile, so the
// caller reports it — but the only walkable route ENDS on that very edge,
// after a detour through Viridian Forest (0x33) that re-enters 0x0D in the
// northern band. A search keyed by map returns simple paths and can never
// express it; one that bars the reported edge everywhere rules out its own
// answer. Measured 2026-08-27: this route planning failed before any walk.
func TestFindRouteAvoidingReEntersTheStartMap(t *testing.T) {
	north := Edge{Kind: EdgeConnection, From: 0x0D, To: 0x02, Dir: 0}
	intoForest := Edge{Kind: EdgeWarp, From: 0x0D, To: 0x33, WarpX: 15, WarpY: 55}
	outOfForest := Edge{Kind: EdgeWarp, From: 0x33, To: 0x0D, WarpX: 1, WarpY: 0}
	g := &Graph{Edges: map[uint8][]Edge{
		0x0D: {north, intoForest},
		0x33: {outOfForest},
		0x02: nil,
	}}

	route, err := FindRouteAvoiding(g, 0x0D, 0x02, map[Edge]bool{north: true})
	if err != nil {
		t.Fatalf("FindRouteAvoiding: %v", err)
	}
	want := []Edge{intoForest, outOfForest, north}
	if len(route) != len(want) {
		t.Fatalf("route = %v, want the 3-hop forest detour %v", route, want)
	}
	for i := range want {
		if route[i] != want[i] {
			t.Fatalf("route[%d] = %+v, want %+v (full route %v)", i, route[i], want[i], route)
		}
	}
}
