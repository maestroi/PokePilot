package skill

import (
	"os"
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

func TestCyclingRoadUphillIsPermanentGate(t *testing.T) {
	for _, edge := range []world.Edge{
		{Kind: world.EdgeConnection, From: route18Map, To: route17Map},
		{Kind: world.EdgeConnection, From: route17Map, To: route16Map},
	} {
		transition := requireTransition(t, edge, "red:cycling_road_uphill", capCanClimbCyclingRoad)
		if !transition.Gate {
			t.Fatalf("uphill Cycling Road edge was modeled as an executable pivot: %+v", transition)
		}
		if !transition.OneWay {
			t.Fatalf("uphill Cycling Road edge missing OneWay: %+v", transition)
		}
	}
	// Downhill remains ordinary topology so Celadon→Fuchsia via Cycling Road
	// still routes once the Bicycle gate warps are satisfied.
	for _, edge := range []world.Edge{
		{Kind: world.EdgeConnection, From: route16Map, To: route17Map},
		{Kind: world.EdgeConnection, From: route17Map, To: route18Map},
	} {
		if transition, ok := redRouteTransitionForEdge(edge); ok && transition.ID == "red:cycling_road_uphill" {
			t.Fatalf("downhill Cycling Road edge was incorrectly uphill-gated: %+v", transition)
		}
	}
	if caps := redRouteCapabilities(nil, new(state.Mem)); caps.Has(capCanClimbCyclingRoad) {
		t.Fatalf("uphill Cycling Road capability must never be projected: %v", caps)
	}
	mem := new(state.Mem)
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = bicycleItem
	mem[sym.BagItems+1] = 1
	if caps := redRouteCapabilities(nil, mem); caps.Has(capCanClimbCyclingRoad) {
		t.Fatalf("owning a Bicycle must not project uphill Cycling Road: %v", caps)
	}
}

// TestRoute18WestGateEscapesToCeladonRoofWithoutUphill locks the stranded
// Route 18 gate pocket measured on run-2isuhypptp3cn1ji08lyq3j6e9: the player
// already owns a Bicycle, stands on the west warp tile (33,8), and needs the
// Celadon Mart roof. Without the uphill gate the static graph prefers
// Route 18→17→16; with it, the first durable escape is through ROUTE_18_GATE
// onto the Fuchsia-facing half, then the ordinary land path. Requires
// POKEMON_RED_ROM.
func TestRoute18WestGateEscapesToCeladonRoofWithoutUphill(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	// Match the farm save's relevant route permissions: Bicycle (gate warps)
	// plus a cleared Route 12 Snorlax so the Fuchsia→Lavender land path is
	// open. Without the land path, routing correctly reports the blocked
	// uphill Cycling Road edge and never reaches the gate-escape assertion.
	var mem state.Mem
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = bicycleItem
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = 0xff
	setEventFlag(&mem, eventBeatRoute12Snorlax)

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if !prereqs.Capabilities.Has(capCanRideCyclingRoad) {
		t.Fatalf("capabilities did not include %q: %v", capCanRideCyclingRoad, prereqs.Capabilities)
	}
	if !prereqs.Capabilities.Has(capCanClearSnorlax) {
		t.Fatalf("capabilities did not include %q: %v", capCanClearSnorlax, prereqs.Capabilities)
	}

	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route18Map, celadonMartRoofMap, 33, 8, int(vendingStandX), int(vendingStandY), nil, prereqs,
	)
	if err != nil {
		t.Fatalf("no route from Route 18 (33,8) to Celadon Mart roof with a Bicycle: %v", err)
	}
	if len(route) == 0 {
		t.Fatal("empty route from Route 18 west gate to Celadon Mart roof")
	}
	sawGateEscape := false
	for i, step := range route {
		e := step.Edge
		if e.Kind == world.EdgeConnection && e.From == route18Map && e.To == route17Map {
			t.Fatalf("leg %d climbed Cycling Road uphill Route 18→17: %+v", i+1, route)
		}
		if e.Kind == world.EdgeConnection && e.From == route17Map && e.To == route16Map {
			t.Fatalf("leg %d climbed Cycling Road uphill Route 17→16: %+v", i+1, route)
		}
		if e.Kind == world.EdgeWarp && e.From == route18Map && e.To == route18Gate1FMap &&
			(e.WarpX == 33 || e.WarpX == 40) {
			sawGateEscape = true
		}
	}
	if !sawGateEscape {
		t.Fatalf("route never crossed Route 18 Gate to leave the west pocket: %+v", route)
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
