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

func TestCyclingRoadModelsOnlyTheBikeCorridor(t *testing.T) {
	east := requireTransition(t,
		world.Edge{Kind: world.EdgeWarp, From: route16Map, To: route16Gate1FMap, WarpX: 24, WarpY: 10},
		"red:route16_snorlax_bicycle", capCanClearSnorlax, capCanRideCyclingRoad)
	if east.Gate {
		t.Fatal("east Route 16 entry must execute the Snorlax action, not be a pure gate")
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

	requireTransition(t,
		world.Edge{Kind: world.EdgeConnection, From: route16Map, To: celadonCityMap},
		"red:route16_snorlax", capCanClearSnorlax)
	if transition, ok := redRouteTransitionForEdge(world.Edge{Kind: world.EdgeConnection, From: celadonCityMap, To: route16Map}); ok && transition.ID == "red:route16_snorlax" {
		t.Fatalf("Celadon -> Route 16 entry was over-gated by Snorlax: %+v", transition)
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

func TestGymCutGatesAreEntryOnly(t *testing.T) {
	requireTransition(t,
		world.Edge{Kind: world.EdgeWarp, From: celadonCityMap, To: celadonGymMap},
		"red:celadon_gym_cut", capCanCut)
	if transition, ok := redRouteTransitionForEdge(world.Edge{Kind: world.EdgeWarp, From: celadonGymMap, To: celadonCityMap}); ok && transition.ID == "red:celadon_gym_cut" {
		t.Fatalf("leaving Celadon Gym was Cut-gated: %+v", transition)
	}

	// Unlike Celadon, the same tree sits on both sides of Vermilion's door in
	// the immutable ROM collision (measured on run-3djisxgsy3dgzpnsde2inzyuh
	// round 7): leaving lands on the untouched yard side exactly as entering
	// starts from the untouched street side, so both directions need the
	// pivot or GoTo reports "world: no route" trying to leave after Surge.
	requireTransition(t,
		world.Edge{Kind: world.EdgeWarp, From: semanticVermilionCityMap, To: vermilionGymMap},
		"red:vermilion_gym_cut", capCanCut)
	requireTransition(t,
		world.Edge{Kind: world.EdgeWarp, From: vermilionGymMap, To: semanticVermilionCityMap},
		"red:vermilion_gym_cut", capCanCut)
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
		world.Edge{Kind: world.EdgeConnection, From: semanticCeruleanCityMap, To: semanticRoute9Map},
		"red:route9_cut", capCanCut)
	if transition.Gate {
		t.Fatalf("red:route9_cut is a Gate: %+v; a static pre-cut component strands Rock Tunnel/Lavender/Celadon behind it", transition)
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
