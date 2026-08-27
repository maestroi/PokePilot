package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestBlockImmediateReverseChoosesForestDetour(t *testing.T) {
	north := world.Edge{Kind: world.EdgeConnection, From: 0x0D, To: 0x02, Dir: 0}
	reverseLeft := world.Edge{Kind: world.EdgeWarp, From: 0x32, To: 0x0D, WarpX: 4, WarpY: 7}
	reverseRight := world.Edge{Kind: world.EdgeWarp, From: 0x32, To: 0x0D, WarpX: 5, WarpY: 7}
	forest := world.Edge{Kind: world.EdgeWarp, From: 0x32, To: 0x33, WarpX: 5, WarpY: 0}
	forestNorth := world.Edge{Kind: world.EdgeWarp, From: 0x33, To: 0x2F, WarpX: 1, WarpY: 0}
	northGate := world.Edge{Kind: world.EdgeWarp, From: 0x2F, To: 0x0D, WarpX: 5, WarpY: 0}
	g := &world.Graph{Edges: map[uint8][]world.Edge{
		0x32: {reverseLeft, reverseRight, forest},
		0x33: {forestNorth},
		0x2F: {northGate},
		0x0D: {north},
		0x02: nil,
	}}

	without, err := world.FindRouteAvoiding(g, 0x32, 0x02, nil)
	if err != nil {
		t.Fatalf("unblocked route: %v", err)
	}
	if len(without) != 2 || without[0] != reverseLeft {
		t.Fatalf("premise: unblocked route = %+v, want immediate reverse then north", without)
	}

	blocked := map[world.Edge]bool{}
	blockImmediateReverse(g, blocked, 0x32, 0x0D)
	if !blocked[reverseLeft] || !blocked[reverseRight] {
		t.Fatalf("paired reverse edges not both blocked: %+v", blocked)
	}
	if blocked[forest] {
		t.Fatalf("forward forest edge was blocked: %+v", blocked)
	}

	got, err := world.FindRouteAvoiding(g, 0x32, 0x02, blocked)
	if err != nil {
		t.Fatalf("route with immediate reverse blocked: %v", err)
	}
	want := []world.Edge{forest, forestNorth, northGate, north}
	if len(got) != len(want) {
		t.Fatalf("route = %+v, want forest detour %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("route[%d] = %+v, want %+v (route %+v)", i, got[i], want[i], got)
		}
	}
}
