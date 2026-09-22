package skill

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestAuditedRouteCapabilitiesProjectBicycle(t *testing.T) {
	mem := new(state.Mem)
	if caps := redRouteCapabilities(nil, mem); caps.Has(capCanRideCyclingRoad) {
		t.Fatalf("empty bag unexpectedly projects %q: %v", capCanRideCyclingRoad, caps)
	}
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = bicycleItem
	mem[sym.BagItems+1] = 1
	if caps := redRouteCapabilities(nil, mem); !caps.Has(capCanRideCyclingRoad) {
		t.Fatalf("Bicycle in bag did not project %q: %v", capCanRideCyclingRoad, caps)
	}
}

func requireTransition(t *testing.T, edge world.Edge, id string, requires ...gameruntime.CapabilityID) gameruntime.Transition {
	t.Helper()
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok {
		t.Fatalf("edge %+v has no semantic transition", edge)
	}
	if transition.ID != id {
		t.Fatalf("transition id = %q, want %q", transition.ID, id)
	}
	if len(transition.Requires) != len(requires) {
		t.Fatalf("requires = %v, want %v", transition.Requires, requires)
	}
	for i := range requires {
		if transition.Requires[i] != requires[i] {
			t.Fatalf("requires = %v, want %v", transition.Requires, requires)
		}
	}
	return transition
}

func TestCeladonInaccessibleMartWarpIsPermanentGate(t *testing.T) {
	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  celadonCityMap,
		To:    celadonMart5FMap,
		WarpX: celadonInaccessibleMartWarpX,
		WarpY: celadonInaccessibleMartWarpY,
	}
	transition := requireTransition(t, edge, "red:celadon_inaccessible_mart_warp", capCanUseInaccessibleWarp)
	if !transition.Gate {
		t.Fatalf("inaccessible Celadon wall warp was modeled as an executable pivot: %+v", transition)
	}
	if caps := redRouteCapabilities(nil, new(state.Mem)); caps.Has(capCanUseInaccessibleWarp) {
		t.Fatalf("inaccessible warp capability must never be projected: %v", caps)
	}

	// The real Game Corner door immediately west remains ordinary topology;
	// only the source warp explicitly marked inaccessible by the Red decomp is
	// suppressed.
	realDoor := world.Edge{Kind: world.EdgeWarp, From: celadonCityMap, To: gameCornerMap, WarpX: 28, WarpY: 19}
	if got, ok := redRouteTransitionForEdge(realDoor); ok && got.ID == "red:celadon_inaccessible_mart_warp" {
		t.Fatalf("real Game Corner warp was suppressed: %+v", got)
	}
}

func TestSilphCo1FInaccessibleStairWarpIsPermanentGate(t *testing.T) {
	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  silphCo1FMap,
		To:    silphCo3FMap,
		WarpX: silphCo1FInaccessibleStairWarpX,
		WarpY: silphCo1FInaccessibleStairWarpY,
	}
	transition := requireTransition(t, edge, "red:silph_co_1f_inaccessible_stair_warp", capCanUseInaccessibleWarp)
	if !transition.Gate {
		t.Fatalf("inaccessible Silph Co 1F stair warp was modeled as an executable pivot: %+v", transition)
	}
	if caps := redRouteCapabilities(nil, new(state.Mem)); caps.Has(capCanUseInaccessibleWarp) {
		t.Fatalf("inaccessible warp capability must never be projected: %v", caps)
	}

	// The real way up remains ordinary topology; only the source warp
	// explicitly marked inaccessible by the Red decomp is suppressed.
	elevator := world.Edge{Kind: world.EdgeWarp, From: silphCo1FMap, To: 0xec, WarpX: 20, WarpY: 0}
	if got, ok := redRouteTransitionForEdge(elevator); ok && got.ID == "red:silph_co_1f_inaccessible_stair_warp" {
		t.Fatalf("real Silph Co elevator warp was suppressed: %+v", got)
	}
}

func TestCyclingRoadModelsOnlyTheBikeCorridor(t *testing.T) {
	east := requireTransition(t,
		world.Edge{Kind: world.EdgeWarp, From: route16Map, To: route16Gate1FMap, WarpX: 24, WarpY: 10},
		"red:route16_snorlax_bicycle", capCanClearSnorlax, capCanRideCyclingRoad)
	if !east.Gate {
		t.Fatal("east Route 16 lower entry must be a Gate so its landing stays the lower corridor")
	}

	west := requireTransition(t,
		world.Edge{Kind: world.EdgeWarp, From: route16Map, To: route16Gate1FMap, WarpX: 17, WarpY: 10},
		"red:cycling_road_bicycle", capCanRideCyclingRoad)
	if !west.Gate {
		t.Fatal("west Route 16 gate should be a pure Bicycle gate")
	}

	// Route 16's upper passage leads to the pedestrian/Fly-house side and is
	// deliberately not part of Cycling Road's lower Bicycle-only corridor.
	upper := world.Edge{Kind: world.EdgeWarp, From: route16Map, To: route16Gate1FMap, WarpX: 24, WarpY: 4}
	if transition, ok := redRouteTransitionForEdge(upper); ok && transition.ID == "red:cycling_road_bicycle" {
		t.Fatalf("upper pedestrian Route 16 passage was Bicycle-gated: %+v", transition)
	}

	requireTransition(t,
		world.Edge{Kind: world.EdgeWarp, From: route18Map, To: route18Gate1FMap, WarpX: 40, WarpY: 8},
		"red:cycling_road_bicycle", capCanRideCyclingRoad)
	requireTransition(t,
		world.Edge{Kind: world.EdgeWarp, From: route18Gate1FMap, To: route18Map, WarpX: 0, WarpY: 4},
		"red:cycling_road_bicycle", capCanRideCyclingRoad)

	snorlaxEdge := requireTransition(t,
		world.Edge{Kind: world.EdgeConnection, From: route16Map, To: celadonCityMap},
		"red:route16_snorlax", capCanClearSnorlax)
	if !snorlaxEdge.PivotOnly || !snorlaxEdge.PortBypass {
		t.Fatalf("route16_snorlax = %+v, want PivotOnly+PortBypass so the east component stays walkable without the flute", snorlaxEdge)
	}
	if transition, ok := redRouteTransitionForEdge(world.Edge{Kind: world.EdgeConnection, From: celadonCityMap, To: route16Map}); ok && transition.ID == "red:route16_snorlax" {
		t.Fatalf("Celadon -> Route 16 entry was over-gated by Snorlax: %+v", transition)
	}
}

func TestCyclingRoadUphillConnectionsAreOrdinaryTopology(t *testing.T) {
	// JoypadOverworld injects PAD_DOWN only when Route 17 has *no* held input.
	// Explicit Up is legal, so both map directions must remain routable. The
	// old never-projected "can_climb_cycling_road" gate encoded an automation
	// release-frame bug as if it were a game rule.
	for _, edge := range []world.Edge{
		{Kind: world.EdgeConnection, From: route18Map, To: route17Map},
		{Kind: world.EdgeConnection, From: route17Map, To: route16Map},
		{Kind: world.EdgeConnection, From: route16Map, To: route17Map},
		{Kind: world.EdgeConnection, From: route17Map, To: route18Map},
	} {
		if transition, ok := redRouteTransitionForEdge(edge); ok && transition.ID == "red:cycling_road_uphill" {
			t.Fatalf("Cycling Road connection %+v still carries obsolete uphill gate: %+v", edge, transition)
		}
	}
}

func TestCyclingRoadAutoDownMatchesROMInputRule(t *testing.T) {
	tests := []struct {
		name          string
		mapID         uint8
		trainerBattle bool
		inputHeld     bool
		want          bool
	}{
		{name: "route17 idle", mapID: route17Map, want: true},
		{name: "route17 explicit direction or button", mapID: route17Map, inputHeld: true, want: false},
		{name: "route17 trainer battle", mapID: route17Map, trainerBattle: true, want: false},
		{name: "route16 idle", mapID: route16Map, want: false},
		{name: "route18 idle", mapID: route18Map, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cyclingRoadAutoDown(tc.mapID, tc.trainerBattle, tc.inputHeld); got != tc.want {
				t.Fatalf("cyclingRoadAutoDown(%#02x, trainer=%v, input=%v) = %v, want %v",
					tc.mapID, tc.trainerBattle, tc.inputHeld, got, tc.want)
			}
		})
	}
}

func TestSouthernSeaRouteRequiresSurf(t *testing.T) {
	for _, edge := range []world.Edge{
		{Kind: world.EdgeConnection, From: route19Map, To: route20Map},
		{Kind: world.EdgeConnection, From: route20Map, To: route19Map},
		{Kind: world.EdgeConnection, From: route20Map, To: cinnabarIslandMap},
		{Kind: world.EdgeConnection, From: cinnabarIslandMap, To: route20Map},
	} {
		requireTransition(t, edge, "red:southern_sea_surf", capCanSurf)
	}
	if transition, ok := redRouteTransitionForEdge(world.Edge{Kind: world.EdgeConnection, From: fuchsiaCityMap, To: route19Map}); ok && transition.ID == "red:southern_sea_surf" {
		t.Fatalf("Fuchsia -> Route 19 beach was incorrectly Surf-gated: %+v", transition)
	}
}

func TestGymCutGatesAreBidirectionalPivots(t *testing.T) {
	// Issue #525 reproduced the same static-component trap at Celadon that
	// Vermilion already handles: a run resumed inside map 0x86 could not route
	// from Erika's gym back to the Center because the immutable city collision
	// still splits the gym landing yard from the street. Both directions must
	// therefore be action pivots, not pure gates.
	for _, edge := range []world.Edge{
		{Kind: world.EdgeWarp, From: celadonCityMap, To: celadonGymMap},
		{Kind: world.EdgeWarp, From: celadonGymMap, To: celadonCityMap},
	} {
		transition := requireTransition(t, edge, "red:celadon_gym_cut", capCanCut)
		if transition.Gate {
			t.Fatalf("Celadon Gym Cut transition was a pure gate instead of a pivot: %+v", transition)
		}
	}

	// The same tree sits on both sides of Vermilion's door in the immutable
	// ROM collision (measured on run-3djisxgsy3dgzpnsde2inzyuh round 7):
	// leaving lands on the untouched yard side exactly as entering starts from
	// the untouched street side, so both directions need the pivot or GoTo
	// reports "world: no route" trying to leave after Surge.
	for _, edge := range []world.Edge{
		{Kind: world.EdgeWarp, From: semanticVermilionCityMap, To: vermilionGymMap},
		{Kind: world.EdgeWarp, From: vermilionGymMap, To: semanticVermilionCityMap},
	} {
		transition := requireTransition(t, edge, "red:vermilion_gym_cut", capCanCut)
		if transition.Gate {
			t.Fatalf("Vermilion Gym Cut transition was a pure gate instead of a pivot: %+v", transition)
		}
	}
}

// TestRoute9CutIsPivotNotGate is the Route 9 sibling of the Vermilion Gym
// fix above: run-1948e1rnco3sp1y9bbhdwp7eov showed the immutable ROM
// collision splits Route 9 into a Cerulean-side component and a Route
// 10-side component joined only by the tree between them, so treating this
// edge as a Gate (static pre-cut landing component) makes "route 9" itself
// reachable but silently strands every destination past it — Rock Tunnel,
// Lavender, and the Route 8/7 Underground Path detour around Saffron. It
// must be a real pivot, exactly like red:vermilion_gym_cut.
func TestRoute9CutIsPivotNotGate(t *testing.T) {
	transition := requireTransition(t,
		world.Edge{Kind: world.EdgeConnection, From: semanticRoute9Map, To: route10Map},
		"red:route9_cut", capCanCut)
	if transition.Gate {
		t.Fatalf("red:route9_cut is a Gate: %+v; a static pre-cut component strands Rock Tunnel/Lavender/Celadon behind it", transition)
	}
	if !transition.PivotOnly || !transition.PortBypass {
		t.Fatalf("red:route9_cut = %+v, want PivotOnly+PortBypass FROM-side bridge", transition)
	}
}

func TestCompoundStoryTargetsStayOutOfJourneyVocabulary(t *testing.T) {
	public := map[string]bool{}
	for _, name := range PlaceNames() {
		public[name] = true
	}
	for _, name := range []string{
		"route 12 snorlax",
		"route 12 south of snorlax",
		"safari zone gate",
		"safari exit approach",
		"safari gold teeth",
		"safari secret house",
		"warden",
		"vermilion dock",
		"ss anne 1f",
		"ss anne 2f",
		"ss anne captain's room",
	} {
		if public[name] {
			t.Errorf("compound story target %q leaked into PlaceNames", name)
		}
		if _, ok := Place(name); !ok {
			t.Errorf("compound story target %q no longer resolves through Place", name)
		}
	}
}
