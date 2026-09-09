package world

import (
	"errors"
	"github.com/maestroi/pokepilot/red/rom"
	"os"
	"testing"
)

func TestDirectedReachabilityDoesNotExpandDestination(t *testing.T) {
	g := &Graph{componentAware: true, Edges: map[uint8][]Edge{1: nil}, comps: map[uint8][][]int{1: {{1}, {0}, {2}}}, reachable: map[uint8]map[int][]int{1: {1: {1, 2}}}}
	if _, err := FindRouteAtDestination(g, 1, 1, 0, 2, 0, 0, nil); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("uphill route: %v", err)
	}
	if _, err := FindRouteAtDestination(g, 1, 1, 0, 0, 0, 2, nil); err != nil {
		t.Fatalf("downhill route: %v", err)
	}
}

func TestRoute4ExitCanReachEasternGrassButNotReturn(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("ROM required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := rom.ParseMap(data, 0x0f)
	if err != nil {
		t.Fatal(err)
	}
	g, err := Build(data, h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FindPath(g, 24, 5, 64, 14, nil); err != nil {
		t.Fatalf("cave exit to eastern grass: %v", err)
	}
	if _, err := FindPath(g, 64, 14, 24, 5, nil); err == nil {
		t.Fatal("uphill ledges must remain impassable")
	}
	graph, err := BuildGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	route, err := FindRouteAtDestination(graph, 0x0f, 0x03, 10, 10, 5, 18, nil)
	if err != nil {
		t.Fatalf("western Route 4 through cave to Cerulean: %v", err)
	}
	for _, id := range []uint8{0x3b, 0x3c, 0x3d} {
		found := false
		for _, e := range route {
			if e.To == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("route must pass cave floor %02x: %+v", id, route)
		}
	}
}
