package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestBlockVisitedMapsChoosesForestDetour(t *testing.T) {
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

	blocked := blockVisitedMaps(g, nil, 0x32, map[uint8]bool{0x0D: true})
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

// TestBlockVisitedMapsDeadEndKeepsHardBans is the Mt Moon Pokemon Center
// case: every warp out of map 0x44 lands back on Route 4, so the reverse ban
// bans the whole map and GoTo dies on "world: no route" one step after the
// router deliberately routed through the building. The ban is a preference,
// so GoTo drops it and re-plans with only the measured bans — which this
// checks stayed intact, since blockVisitedMaps must not mutate them.
func TestBlockVisitedMapsDeadEndKeepsHardBans(t *testing.T) {
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
	preferred := blockVisitedMaps(g, hard, center, map[uint8]bool{route4: true})

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

// TestForcedRevisitBanClosesDeadEndConnection is the Cerulean City <-> Route 4
// bounce measured on run-11cb1z9tng81m1nslvpe4yp65m: replayed directly from
// that run's failure save state, GoTo left the Route 4 dead-end pocket at
// (89,10), wandered Cerulean's other edges, then crossed back into the same
// pocket from a DIFFERENT Cerulean tile (9,12) than the one it first used
// (0,18). A tile-scoped ban (legAt) never matches a fresh tile, so nothing
// stopped the second crossing until the guard's exact-position repeat fired,
// 18 hops in. forcedRevisitBan instead looks at what the planner is about to
// do: if dropping the "don't revisit" preference forces the very next hop
// back into an already-visited map, that is measured evidence of a genuine
// dead end, banned by (edge, origin map) so every tile of that origin closes
// at once — not just the one tile that happened to be tried first.
func TestForcedRevisitBanClosesDeadEndConnection(t *testing.T) {
	cerulean := uint8(0x03)
	route4 := uint8(0x0F)
	crossing := world.Edge{Kind: world.EdgeConnection, From: cerulean, To: route4, Dir: 0}
	onward := world.Edge{Kind: world.EdgeConnection, From: cerulean, To: 0x02, Dir: 1}
	g := &world.Graph{Edges: map[uint8][]world.Edge{
		cerulean: {crossing, onward},
		route4:   {{Kind: world.EdgeConnection, From: route4, To: cerulean, Dir: 2}},
	}}
	visited := map[uint8]bool{route4: true} // this call already departed Route 4
	deadEnds := map[legFromMap]bool{}

	// Premise: without the fix, a route planned around only hard bans is
	// free to send the walker straight back into the map it just left.
	retry := []world.RouteStep{{Edge: crossing}}
	forced, ok := forcedRevisitBan(g, retry, nil, visited, deadEnds)
	if !ok {
		t.Fatal("forced re-entry into a visited map was not detected")
	}
	want := legFromMap{e: crossing, m: cerulean}
	if forced != want {
		t.Fatalf("ban key = %+v, want %+v", forced, want)
	}

	deadEnds[forced] = true

	// A second attempt, even from ANY other tile of the same origin map
	// crossing the same edge, must now be recognized as already banned:
	// this is what closes the (9,12) crossing after (0,18) was learned.
	if _, ok := forcedRevisitBan(g, retry, nil, visited, deadEnds); ok {
		t.Fatal("already-banned edge was offered again")
	}

	// Cerulean's other edge (not a revisit) must stay untouched.
	if forced, ok := forcedRevisitBan(g, []world.RouteStep{{Edge: onward}}, nil, visited, deadEnds); ok {
		t.Fatalf("a non-revisiting edge was banned: %+v", forced)
	}
}

// TestForcedRevisitBanKeepsOnlyExit is the Route 24 case measured on
// run-3w2ibusy813gfmnierudpllie round 6: Route 24 has exactly two edges, one
// already a real dead end. Banning the other — its only remaining exit —
// would seal it shut for the rest of the call, which can never be correct:
// the player got there somehow, and the same way back out must stay legal.
func TestForcedRevisitBanKeepsOnlyExit(t *testing.T) {
	route24 := uint8(0x18)
	cerulean := uint8(0x03)
	toCerulean := world.Edge{Kind: world.EdgeConnection, From: route24, To: cerulean, Dir: 0}
	toRoute25 := world.Edge{Kind: world.EdgeConnection, From: route24, To: 0x19, Dir: 1}
	g := &world.Graph{Edges: map[uint8][]world.Edge{route24: {toCerulean, toRoute25}}}
	visited := map[uint8]bool{cerulean: true}
	deadEnds := map[legFromMap]bool{{e: toRoute25, m: route24}: true} // Route 25 already a measured dead end

	if forced, ok := forcedRevisitBan(g, []world.RouteStep{{Edge: toCerulean}}, nil, visited, deadEnds); ok {
		t.Fatalf("sealed Route 24's only remaining exit: %+v", forced)
	}
}
