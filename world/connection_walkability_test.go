package world

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestConnectionExitWalkableRejectsOnlyProvenPaddingBands(t *testing.T) {
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
		connections: make(map[Edge]rom.Connection),
		exitComps:   make(map[Edge][]int),
		entryComps:  make(map[Edge][]int),
	}

	edges := g.connectionEdges(1, rom.Connection{Dir: dirNorth, MapID: 2})
	if len(edges) != 4 {
		t.Fatalf("connectionEdges produced %d edges, want 4: %+v", len(edges), edges)
	}

	want := map[[2]int]bool{
		{0, 1}: true,
		{2, 3}: true,
		{4, 4}: false,
		{5, 5}: true,
	}
	for _, edge := range edges {
		start, end, ok := ConnectionBand(edge)
		if !ok {
			t.Fatalf("edge is not band-scoped: %+v", edge)
		}
		wantWalkable, ok := want[[2]int{start, end}]
		if !ok {
			t.Fatalf("unexpected band %d..%d", start, end)
		}
		if got := g.ConnectionExitWalkable(edge); got != wantWalkable {
			t.Errorf("ConnectionExitWalkable(%d..%d) = %v, want %v", start, end, got, wantWalkable)
		}
	}
}

func TestConnectionExitWalkableStaysPermissiveWithoutGeometry(t *testing.T) {
	edge := Edge{Kind: EdgeConnection, From: 1, To: 2}
	g := &Graph{}
	if !g.ConnectionExitWalkable(edge) {
		t.Fatal("graph without component evidence treated connection as proven unwalkable")
	}
}
