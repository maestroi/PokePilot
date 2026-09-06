package world

import (
	"errors"
	"testing"
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

func TestWithMapGridRejectsWrongMap(t *testing.T) {
	g := &Graph{componentAware: true}
	_, err := g.WithMapGrid(1, &Grid{MapID: 2})
	if err == nil {
		t.Fatal("WithMapGrid accepted a grid from a different map")
	}
}
