package world

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestConnectionEdgesSplitContiguousComponentPairs(t *testing.T) {
	g := &Graph{
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
		connections: make(map[Edge]rom.Connection),
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
		got := g.entryComps[e]
		if len(got) != 1 {
			t.Fatalf("band %d..%d entry components = %v, want exactly one", start, end, got)
		}
		entries[got[0]] = true
	}
	if len(entries) < 2 {
		t.Fatalf("Cerulean -> Route 4 bands land in only %d component(s): %v; want multiple", len(entries), entries)
	}
}
