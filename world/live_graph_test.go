package world

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func TestWithMapGridRecomputesReachabilityWithoutMutatingBase(t *testing.T) {
	edge := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	base := &Graph{
		Edges: map[uint8][]Edge{
			1: {edge},
			2: nil,
		},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1, 0, 2}}, // closed middle tile splits west/east halves
			2: {{1}},
		},
		exitComps:  map[Edge][]int{edge: {2}},
		entryComps: map[Edge][]int{edge: {1}},
		tiles: map[uint8]dim{
			1: {w: 3, h: 1},
			2: {w: 1, h: 1},
		},
	}

	if _, err := FindRouteAt(base, 1, 2, 0, 0, nil); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("base route error = %v, want ErrNoRoute", err)
	}

	liveGrid := &Grid{
		MapID:    1,
		Width:    3,
		Height:   1,
		walkable: []bool{true, true, true}, // runtime-opened middle tile
	}
	live, err := base.WithMapGrid(1, liveGrid)
	if err != nil {
		t.Fatalf("WithMapGrid: %v", err)
	}
	route, err := FindRouteAt(live, 1, 2, 0, 0, nil)
	if err != nil {
		t.Fatalf("live FindRouteAt: %v", err)
	}
	if len(route) != 1 || route[0] != edge {
		t.Fatalf("live route = %#v, want [%#v]", route, edge)
	}

	// The overlay is one loaded-map observation, not learned geometry.
	if _, err := FindRouteAt(base, 1, 2, 0, 0, nil); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("base graph changed after live overlay: error = %v, want ErrNoRoute", err)
	}
}

func TestWithMapGridResegmentsConnectionsTouchingLiveMap(t *testing.T) {
	base := &Graph{
		Edges:          make(map[uint8][]Edge),
		componentAware: true,
		comps: map[uint8][][]int{
			1: {
				{1, 1, 1, 1},
				{1, 1, 1, 1},
			},
			2: {
				{1, 1, 1, 1},
				{1, 1, 1, 1},
			},
		},
		tiles: map[uint8]dim{
			1: {w: 4, h: 2},
			2: {w: 4, h: 2},
		},
		connections: make(map[Edge]worldmodel.Connection),
		exitComps:   make(map[Edge][]int),
		entryComps:  make(map[Edge][]int),
		reachable:   make(map[uint8]map[int][]int),
	}

	to2 := worldmodel.Connection{Dir: dirNorth, MapID: 2}
	to1 := worldmodel.Connection{Dir: dirSouth, MapID: 1}
	base.Edges[1] = base.connectionEdges(1, to2)
	base.Edges[2] = base.connectionEdges(2, to1)
	if len(base.Edges[1]) != 1 || len(base.Edges[2]) != 1 {
		t.Fatalf("base connection bands = 1->2 %v, 2->1 %v; want one each", base.Edges[1], base.Edges[2])
	}
	for _, edges := range base.Edges {
		for _, e := range edges {
			base.exitComps[e] = base.exitPortComps(e)
			base.entryComps[e] = base.expandComponents(e.To, base.entryPortComps(e))
		}
	}

	// Runtime geometry splits map 2 into left/right components at x=2. Both
	// the incoming 1->2 connection (entry components changed) and the outgoing
	// 2->1 connection (exit components changed) must be re-segmented.
	liveGrid := &Grid{
		MapID:  2,
		Width:  4,
		Height: 2,
		walkable: []bool{
			true, true, false, true,
			true, true, false, true,
		},
	}
	live, err := base.WithMapGrid(2, liveGrid)
	if err != nil {
		t.Fatalf("WithMapGrid: %v", err)
	}

	assertBands := func(name string, edges []Edge) {
		t.Helper()
		if len(edges) != 3 {
			t.Fatalf("%s produced %d bands, want 3: %+v", name, len(edges), edges)
		}
		want := [][2]int{{0, 1}, {2, 2}, {3, 3}}
		for i, e := range edges {
			start, end, ok := ConnectionBand(e)
			if !ok || start != want[i][0] || end != want[i][1] {
				t.Fatalf("%s band %d = %d..%d scoped=%t, want %d..%d",
					name, i, start, end, ok, want[i][0], want[i][1])
			}
		}
	}
	assertBands("1->2", live.Edges[1])
	assertBands("2->1", live.Edges[2])

	// The blocked middle seam run may remain represented for diagnostics, but
	// it must have no exit component and therefore cannot be selected.
	if got := live.exitComps[live.Edges[2][1]]; len(got) != 0 {
		t.Fatalf("blocked live seam band exit components = %v, want none", got)
	}
	if live.ConnectionExitWalkable(live.Edges[2][1]) {
		t.Fatal("blocked live seam band is selectable")
	}

	// A destination in map 2's right-hand component must select only the
	// right-hand live band, not the old aggregate 0..3 band.
	route, err := FindRouteAtDestination(live, 1, 2, 0, 0, 3, 1, nil)
	if err != nil {
		t.Fatalf("FindRouteAtDestination: %v", err)
	}
	if len(route) != 1 {
		t.Fatalf("route = %+v, want one connection edge", route)
	}
	start, end, ok := ConnectionBand(route[0])
	if !ok || start != 3 || end != 3 {
		t.Fatalf("selected live band = %d..%d scoped=%t, want 3..3", start, end, ok)
	}

	// Live segmentation is snapshot-local.
	if len(base.Edges[1]) != 1 || len(base.Edges[2]) != 1 {
		t.Fatalf("base graph mutated: 1->2 %v, 2->1 %v", base.Edges[1], base.Edges[2])
	}
}

func TestWithMapGridRejectsWrongMap(t *testing.T) {
	g := &Graph{componentAware: true}
	_, err := g.WithMapGrid(1, &Grid{MapID: 2})
	if err == nil {
		t.Fatal("WithMapGrid accepted a grid from a different map")
	}
}
