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

	blocked := blockImmediateReverse(g, nil, 0x32, 0x0D)
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

// TestBlockImmediateReverseDeadEndKeepsHardBans is the Mt Moon Pokemon Center
// case: every warp out of map 0x44 lands back on Route 4, so the reverse ban
// bans the whole map and GoTo dies on "world: no route" one step after the
// router deliberately routed through the building. The ban is a preference,
// so GoTo drops it and re-plans with only the measured bans — which this
// checks stayed intact, since blockImmediateReverse must not mutate them.
func TestBlockImmediateReverseDeadEndKeepsHardBans(t *testing.T) {
	route4 := uint8(0x0F)
	center := uint8(0x44)
	left := world.Edge{Kind: world.EdgeWarp, From: center, To: route4, WarpX: 3, WarpY: 7}
	right := world.Edge{Kind: world.EdgeWarp, From: center, To: route4, WarpX: 4, WarpY: 7}
	onward := world.Edge{Kind: world.EdgeConnection, From: route4, To: 0x03, Dir: 3}
	g := &world.Graph{Edges: map[uint8][]world.Edge{
		center: {left, right},
		route4: {onward},
		0x03:   nil,
	}}

	hard := map[world.Edge]bool{onward: true} // a measured-unwalkable leg
	preferred := blockImmediateReverse(g, hard, center, route4)

	if len(hard) != 1 || !hard[onward] {
		t.Fatalf("hard bans were mutated: %+v", hard)
	}
	if !preferred[left] || !preferred[right] || !preferred[onward] {
		t.Fatalf("preferred = %+v, want both reverses plus the hard ban", preferred)
	}
	if _, err := world.FindRouteAvoiding(g, center, 0x03, preferred); err == nil {
		t.Fatal("premise: dead-end map still routed with every exit banned")
	}
	// Dropping only the preference must free the exit; the hard ban applies
	// to the first hop only, so the onward leg is legal from Route 4.
	got, err := world.FindRouteAvoiding(g, center, 0x03, hard)
	if err != nil {
		t.Fatalf("route with the reverse preference dropped: %v", err)
	}
	if len(got) != 2 || got[0] != left || got[1] != onward {
		t.Fatalf("route = %+v, want out of the center then onward", got)
	}
}
