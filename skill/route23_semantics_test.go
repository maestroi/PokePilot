package skill

import (
	"errors"
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestRoute23VictoryRoadEntranceIsSemanticPivot(t *testing.T) {
	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  route23Map,
		To:    victoryRoad1FMap,
		WarpX: route23VictoryRoadWarpX,
		WarpY: route23VictoryRoadWarpY,
	}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok {
		t.Fatal("Route 23 -> Victory Road 1F entrance is missing semantic transition")
	}
	if transition.ID != "red:route23_league_approach" {
		t.Fatalf("transition id=%q", transition.ID)
	}
	if transition.Gate || transition.PivotOnly {
		t.Fatalf("League approach must be an action pivot, got gate=%v pivot_only=%v", transition.Gate, transition.PivotOnly)
	}
	if len(transition.Requires) != 2 || transition.Requires[0] != capCanSurf || transition.Requires[1] != capCanPassRoute23BadgeChecks {
		t.Fatalf("requirements=%v, want Surf + Route 23 badge checks", transition.Requires)
	}
}

func TestRoute23SouthReturnIsSemanticPivot(t *testing.T) {
	for _, x := range []uint8{7, 8} {
		edge := world.Edge{
			Kind:  world.EdgeWarp,
			From:  route23Map,
			To:    route22GateMap,
			WarpX: x,
			WarpY: route23Route22GateWarpY,
		}
		transition, ok := redRouteTransitionForEdge(edge)
		if !ok {
			t.Fatalf("Route 23 south warp x=%d is missing semantic return transition", x)
		}
		if transition.ID != "red:route23_league_return" {
			t.Fatalf("x=%d transition id=%q", x, transition.ID)
		}
		if transition.Gate || transition.PivotOnly || transition.PortBypass {
			t.Fatalf("League return must be a source action pivot, got gate=%v pivot_only=%v port_bypass=%v",
				transition.Gate, transition.PivotOnly, transition.PortBypass)
		}
		if len(transition.Requires) != 2 || transition.Requires[0] != capCanSurf || transition.Requires[1] != capCanPassRoute23BadgeChecks {
			t.Fatalf("x=%d requirements=%v, want Surf + Route 23 badge checks", x, transition.Requires)
		}
	}

	// Entering Route 23 from the gate remains ordinary geometry. The semantic
	// action exists only to bridge the north-side water-separated component
	// toward the south exit.
	reverse := world.Edge{
		Kind:  world.EdgeWarp,
		From:  route22GateMap,
		To:    route23Map,
		WarpX: 4,
		WarpY: 0,
	}
	if transition, ok := redRouteTransitionForEdge(reverse); ok && transition.ID == "red:route23_league_return" {
		t.Fatalf("Route 22 Gate -> Route 23 unexpectedly owns League return action: %+v", transition)
	}
}

func TestRoute23LeagueReturnNorthComponentDiscriminator(t *testing.T) {
	for _, tc := range []struct {
		x, y uint8
		want bool
	}{
		{14, 31, true},  // 2F exit door
		{18, 30, true},  // north exterior
		{14, 37, true},  // east exit pocket
		{4, 31, false},  // 1F entrance door
		{4, 32, false},  // south cave component
		{7, 138, false}, // Route 22 gate approach
	} {
		if got := route23LeagueReturnNeedsVictoryRoad(tc.x, tc.y); got != tc.want {
			t.Errorf("Route 23 (%d,%d) north component=%v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestRoute23LeagueReturnPivotUnavailableFromIndigoSide(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mapID, x, y uint8
		want        bool
	}{
		{name: "north exit door", mapID: route23Map, x: 14, y: 31, want: false},
		{name: "north exterior", mapID: route23Map, x: 18, y: 30, want: false},
		{name: "south cave door", mapID: route23Map, x: 4, y: 31, want: true},
		{name: "south surf approach", mapID: route23Map, x: 8, y: 80, want: true},
		// Approach may still attach at the south gate (Surf north to the League).
		// Return must not: see TestRoute23LeagueReturnOrdinaryAtSouthGate.
		{name: "route 22 gate approach", mapID: route23Map, x: 7, y: 138, want: true},
		{name: "indigo exterior", mapID: indigoPlateauMap, want: false},
		{name: "indigo lobby", mapID: indigoPlateauLobbyMap, want: false},
		{name: "unrelated map", mapID: semanticVermilionCityMap, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := route23SurfPivotAvailable(nil, tc.mapID, tc.x, tc.y); got != tc.want {
				t.Fatalf("approach pivot available=%v, want %v", got, tc.want)
			}
		})
	}
}

// TestRoute23LeagueReturnOrdinaryAtSouthGate pins run-3w50i60f631u41je7v36wsudzt:
// a player already south of every Surf band must see the Route 22 Gate warps as
// ordinary geometry. Attaching red:route23_league_return there required Surf +
// seven badge checks and soft-locked Boulder-only parties out of Kanto.
func TestRoute23LeagueReturnOrdinaryAtSouthGate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mapID, x, y uint8
		want        bool
	}{
		{name: "south gate player", mapID: route23Map, x: 9, y: 137, want: false},
		{name: "route 22 gate approach", mapID: route23Map, x: 7, y: 138, want: false},
		{name: "just south of last water band", mapID: route23Map, x: 8, y: 102, want: false},
		{name: "on southernmost barrier row", mapID: route23Map, x: 8, y: 101, want: true},
		{name: "between surf bands", mapID: route23Map, x: 8, y: 80, want: true},
		{name: "south cave door", mapID: route23Map, x: 4, y: 31, want: true},
		{name: "north exit door", mapID: route23Map, x: 14, y: 31, want: false},
		{name: "indigo lobby", mapID: indigoPlateauLobbyMap, want: false},
		{name: "unrelated map", mapID: semanticVermilionCityMap, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := route23LeagueReturnPivotAvailable(nil, tc.mapID, tc.x, tc.y); got != tc.want {
				t.Fatalf("return pivot available=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestRoute23SurfPivotUnavailableFromVictoryRoad2FExitSide(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatalf("route graph: %v", err)
	}
	for _, tc := range []struct {
		name string
		x, y uint8
		want bool
	}{
		{name: "exit door", x: 29, y: 7, want: false},
		{name: "3f east ladder", x: 27, y: 7, want: false},
		{name: "1f ladder landing", x: 0, y: 8, want: true},
		{name: "3f west ladder", x: 23, y: 7, want: true},
		{name: "sealed 3f ladder pocket", x: 25, y: 14, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := route23SurfPivotAvailable(g, victoryRoad2FMap, tc.x, tc.y); got != tc.want {
				t.Fatalf("pivot available=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestRoute23NorthVictoryRoadExitIsNotCollapsedIntoLeagueApproach(t *testing.T) {
	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  route23Map,
		To:    victoryRoad2FMap,
		WarpX: 14,
		WarpY: 31,
	}
	transition, ok := redAuditedRouteTransitionForEdge(edge)
	if ok && transition.ID == "red:route23_league_approach" {
		t.Fatal("north Route 23 <-> Victory Road 2F exit must remain ordinary geometry")
	}
}

// TestRoute23SouthGateLeavesWithoutSurf is the routing regression for
// run-3w50i60f631u41je7v36wsudzt: from the south gate pocket with only the
// Boulder Badge, Viridian must price as an ordinary walking route. The Catch
// objective that exposed the soft-lock still cannot reach grass without Surf;
// escaping south is the recoverable postcondition.
func TestRoute23SouthGateLeavesWithoutSurf(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatalf("route graph: %v", err)
	}
	var mem state.Mem
	mem[sym.CurMap], mem[sym.XCoord], mem[sym.YCoord] = route23Map, 9, 137
	mem[sym.ObtainedBadges] = 1 << state.BadgeBoulder
	prereqs := redRoutePrerequisites(g, romData, &mem)
	for edge, tr := range prereqs.Transitions {
		if tr.ID == "red:route23_league_return" && edge.From == route23Map {
			t.Fatalf("south gate still attaches %s on warp (%d,%d)", tr.ID, edge.WarpX, edge.WarpY)
		}
	}
	viridian := uint8(0x01)
	if _, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route23Map, viridian, 9, 137, -1, -1, nil, prereqs); err != nil {
		t.Fatalf("Viridian from south gate without Surf: %v", err)
	}
}

// TestIndigoSidePricesKantoAsUnreachable is the regression for
// run-1nnzti2l332xm2pdodobrapjlh. After a Lorelei blackout at Indigo, the
// northbound Route 23 Surf action still bridged the sealed Victory Road exit
// pocket to the 1F door, so every Kanto mart priced as replan-reachable, and
// EnsureItemStock walked to (14,32) and failed. Once the pocket honestly has
// no walking route, the diagnosis must not blame Lance's exit either: the
// Champion's side is a dead end that cannot lead back to Kanto.
//
// Victory Road 2F's exit door reaches Route 23 only through the same pocket
// (run-1gzyx3twnqj6d3hz6yzt02io3g stood there and looped "buy 1 POTION" ->
// no_route), so it must price Kanto the same way, in both routers.
func TestIndigoSidePricesKantoAsUnreachable(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatalf("route graph: %v", err)
	}
	cinnabarMart := uint8(0xAC)
	for _, start := range []struct {
		name        string
		mapID, x, y uint8
	}{
		{"indigo lobby", indigoPlateauLobbyMap, 7, 7},
		{"indigo exterior", indigoPlateauMap, 9, 6},
		{"route 23 exit pocket", route23Map, 14, 32},
		{"victory road 2f exit door", victoryRoad2FMap, 29, 7},
	} {
		t.Run(start.name, func(t *testing.T) {
			var mem state.Mem
			mem[sym.CurMap], mem[sym.XCoord], mem[sym.YCoord] = start.mapID, start.x, start.y
			prereqs := redRoutePrerequisites(g, romData, &mem)
			// Everything but Fly: no walking capability crosses the pocket.
			prereqs.Capabilities = gameruntime.NewCapabilitySet(
				capCanSurf, capCanMoveBoulders, capCanPassRoute23BadgeChecks, capCanCut)
			_, err := world.FindRoutePlanAtDestinationWithCapabilities(
				g, start.mapID, cinnabarMart, int(start.x), int(start.y), -1, -1, nil, prereqs)
			var blocked *world.RouteBlockedError
			switch {
			case errors.Is(err, world.ErrRouteReplanRequired) || err == nil:
				t.Fatalf("Cinnabar Mart priced reachable from the Indigo side: %v", err)
			case errors.As(err, &blocked):
				t.Fatalf("dead-end frontier reported as the blocker: %v", err)
			case !errors.Is(err, world.ErrNoRoute):
				t.Fatalf("err = %v, want world.ErrNoRoute", err)
			}
			weighted, err := world.FindWeightedRoutePlanAtDestinationWithCapabilities(
				g, start.mapID, cinnabarMart, int(start.x), int(start.y), -1, -1, nil, prereqs, world.DefaultRouteCostPolicy())
			if err == nil || errors.Is(err, world.ErrRouteReplanRequired) {
				t.Fatalf("weighted router priced Cinnabar Mart from the Indigo side: %v %+v", err, weighted.Steps)
			}
		})
	}
}
