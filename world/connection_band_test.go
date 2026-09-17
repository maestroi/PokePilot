package world

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

func TestConnectionEdgesSplitContiguousComponentPairs(t *testing.T) {
	g := &Graph{
		Edges:          make(map[uint8][]Edge),
		componentAware: true,
		comps: map[uint8][][]int{
			1: {
				{1, 1, 1, 1, 1, 1},
				{1, 1, 1, 1, 1, 1},
			},
			2: {
				{2, 2, 3, 3, 0, 4},
				{2, 2, 3, 3, 0, 4},
			},
		},
		tiles: map[uint8]dim{
			1: {w: 6, h: 2},
			2: {w: 6, h: 2},
		},
		connections: make(map[Edge]worldmodel.Connection),
		exitComps:   make(map[Edge][]int),
		entryComps:  make(map[Edge][]int),
	}
	c := rom.Connection{Dir: dirNorth, MapID: 2}
	edges := g.connectionEdges(1, c)
	if len(edges) != 4 {
		t.Fatalf("connectionEdges produced %d edges, want 4: %+v", len(edges), edges)
	}

	want := [][2]int{{0, 1}, {2, 3}, {4, 4}, {5, 5}}
	wantExit := [][]int{{1}, {1}, nil, {1}}
	wantEntry := [][]int{{2}, {3}, nil, {4}}
	for i, e := range edges {
		start, end, ok := ConnectionBand(e)
		if !ok {
			t.Fatalf("edge %d is not scoped: %+v", i, e)
		}
		if start != want[i][0] || end != want[i][1] {
			t.Errorf("edge %d band = %d..%d, want %d..%d", i, start, end, want[i][0], want[i][1])
		}
		gotExit := g.connectionPortComps(e, false)
		wantExitComps := wantExit[i]
		if len(gotExit) != len(wantExitComps) || (len(gotExit) == 1 && gotExit[0] != wantExitComps[0]) {
			t.Errorf("edge %d exit components = %v, want %v", i, gotExit, wantExitComps)
		}
		got := g.connectionPortComps(e, true)
		wantComps := wantEntry[i]
		if len(got) != len(wantComps) || (len(got) == 1 && got[0] != wantComps[0]) {
			t.Errorf("edge %d entry components = %v, want %v", i, got, wantComps)
		}
		g.exitComps[e] = gotExit
		g.entryComps[e] = got
	}

	// The map-level destination is identical for every generated edge. The
	// component-aware router must nevertheless choose the band that actually
	// lands in the destination tile's component instead of the first border
	// edge it sees.
	g.Edges[1] = edges
	g.Edges[2] = nil
	route, err := FindRouteAtDestination(g, 1, 2, 0, 0, 2, 0, nil)
	if err != nil {
		t.Fatalf("FindRouteAtDestination: %v", err)
	}
	if len(route) != 1 {
		t.Fatalf("route length = %d, want 1: %+v", len(route), route)
	}
	start, end, ok := ConnectionBand(route[0])
	if !ok || start != 2 || end != 3 {
		t.Fatalf("selected band = %d..%d scoped=%t, want 2..3", start, end, ok)
	}
}

func TestBuildGraphSplitsCeruleanRoute4BorderByLandingComponent(t *testing.T) {
	g := loadGraph(t)
	var edges []Edge
	for _, e := range g.Edges[0x03] {
		if e.Kind == EdgeConnection && e.To == 0x0f {
			edges = append(edges, e)
		}
	}
	if len(edges) < 2 {
		t.Fatalf("Cerulean -> Route 4 has %d connection edge(s), want multiple component-scoped bands", len(edges))
	}

	entries := map[int]bool{}
	for _, e := range edges {
		start, end, ok := ConnectionBand(e)
		if !ok {
			t.Fatalf("Cerulean -> Route 4 edge is not band-scoped: %+v", e)
		}
		if end < start {
			t.Fatalf("invalid band %d..%d on %+v", start, end, e)
		}
		// Check the immediate seam landing, not g.entryComps. BuildGraph expands
		// entryComps through directed movement (for example Route 4's one-way
		// ledges), so one literal landing component can correctly become a set
		// such as [2 1 4] without this border band aggregating multiple tiles.
		got := g.entryPortComps(e)
		if len(got) > 1 {
			t.Fatalf("band %d..%d aggregates multiple raw entry components %v", start, end, got)
		}
		if len(got) == 1 {
			entries[got[0]] = true
		}
	}
	if len(entries) < 2 {
		t.Fatalf("Cerulean -> Route 4 walkable bands land in only %d raw component(s): %v; want multiple", len(entries), entries)
	}
}
