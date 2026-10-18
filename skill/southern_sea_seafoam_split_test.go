package skill

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestCinnabarToFuchsiaDoesNotBridgeRoute20SeafoamSplit locks the shared
// invariant behind run-jc853qns2lmc109wx2hs96btk: with Surf usable, Route 20's
// water topology overlaid, and Cinnabar still on its live land overlay (the
// NewRoutePlanner shape), a multi-band southern-sea PortBypass must not invent
// a completed Cinnabar -> Route 20 -> Route 19 -> Fuchsia plan across the
// Seafoam split. Full PortBypass stays a live-topology frontier per band.
func TestCinnabarToFuchsiaDoesNotBridgeRoute20SeafoamSplit(t *testing.T) {
	romData := badgeFourROM(t)

	mem := fieldTestMem(FieldSurf, true, true, true)
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade |
		1<<state.BadgeThunder | 1<<state.BadgeRainbow | 1<<state.BadgeSoul |
		1<<state.BadgeMarsh | 1<<state.BadgeVolcano | 1<<state.BadgeEarth

	g, err := routeGraphForROM(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	h, err := rom.ParseMap(romData, cinnabarIslandMap)
	if err != nil {
		t.Fatalf("parse Cinnabar: %v", err)
	}
	land, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("build Cinnabar land grid: %v", err)
	}
	g, err = g.WithMapGrid(cinnabarIslandMap, land)
	if err != nil {
		t.Fatalf("overlay Cinnabar land: %v", err)
	}
	g, err = withSurfSeaTopology(g, romData, mem)
	if err != nil {
		t.Fatalf("withSurfSeaTopology: %v", err)
	}
	if mode, ok := g.MapTraversal(cinnabarIslandMap); !ok || mode != world.TraversalLand {
		t.Fatalf("Cinnabar traversal = (%v,%v), want live land overlay", mode, ok)
	}
	if mode, ok := g.MapTraversal(route20Map); !ok || mode != world.TraversalWater {
		t.Fatalf("Route 20 traversal = (%v,%v), want water overlay", mode, ok)
	}

	prereqs := redRoutePrerequisites(g, romData, mem)
	fuchsia, ok := Place("fuchsia city")
	if !ok {
		t.Fatal("fuchsia city place missing")
	}
	route, err := findRoutePlanForDestination(
		g, cinnabarIslandMap, 11, 12, fuchsia, nil, prereqs,
	)
	if err == nil {
		for i, step := range route {
			if step.Edge.From == route20Map && step.Edge.To == route19Map {
				t.Fatalf("completed plan bridges Route 20 Seafoam split at leg %d: %+v", i, route)
			}
			if step.Edge.To == seafoam1FMap {
				t.Fatalf("completed plan enters Seafoam as a southern-sea bridge at leg %d: %+v", i, route)
			}
		}
		return
	}
	if !errors.Is(err, world.ErrRouteReplanRequired) {
		t.Fatalf("Cinnabar -> Fuchsia: %v", err)
	}
	if len(route) == 0 {
		t.Fatal("replan frontier returned empty prefix")
	}
	for i, step := range route {
		if step.Edge.From == route20Map && step.Edge.To == route19Map {
			t.Fatalf("replan prefix bridges Route 20 Seafoam split at leg %d: %+v", i, route)
		}
	}
	// A Surf frontier is fine; a multi-hop prefix that already claims Route 19
	// from Cinnabar is the Seafoam-split teleport.
	if len(route) > 1 {
		t.Fatalf("Surf replan prefix has %d legs (want a single shore frontier): %+v", len(route), route)
	}
}

// TestWeightedCinnabarToFuchsiaPrefersRoute21OverSeafoam locks
// run-186f94od127ou3it5xlpp1lz4g / triage 2df6d0aa9ce33129: TravelCostFastest
// frontier ranking must not prefer Cinnabar's nearer Route 20 Surf shore when
// that landing is a Seafoam dead end for Fuchsia. Map-level hop counts still
// see Route 20 -> Route 19; component-aware landing hops must prefer Route 21.
func TestWeightedCinnabarToFuchsiaPrefersRoute21OverSeafoam(t *testing.T) {
	romData := badgeFourROM(t)

	mem := fieldTestMem(FieldSurf, true, true, true)
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade |
		1<<state.BadgeThunder | 1<<state.BadgeRainbow | 1<<state.BadgeSoul |
		1<<state.BadgeMarsh | 1<<state.BadgeVolcano | 1<<state.BadgeEarth
	setRouteTestEvent(mem, state.EventGotPokedex)
	cut, ok := FieldMoveSpecFor(FieldCut)
	if !ok {
		t.Fatal("Cut field move missing")
	}
	mem[sym.PartyMon1+sym.MonMoves+1] = cut.MoveID
	// Flute is required for any mainland path that touches Route 12/16 Snorlax;
	// without it both Surf shores look equally unroutable and reach-cost alone
	// prefers the nearer Route 20 dead end.
	setTestBag(mem, [2]uint8{hm03SurfItem, 1}, [2]uint8{cut.HMItem, 1}, [2]uint8{0x49, 1})

	g, err := routeGraphForROM(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	h, err := rom.ParseMap(romData, cinnabarIslandMap)
	if err != nil {
		t.Fatalf("parse Cinnabar: %v", err)
	}
	land, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("build Cinnabar land grid: %v", err)
	}
	g, err = g.WithMapGrid(cinnabarIslandMap, land)
	if err != nil {
		t.Fatalf("overlay Cinnabar land: %v", err)
	}
	g, err = withSurfSeaTopology(g, romData, mem)
	if err != nil {
		t.Fatalf("withSurfSeaTopology: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, mem)
	for _, capability := range []gameruntime.CapabilityID{
		capCanSurf, capCanCut, capCanLeaveViridianNorth, capCanClearSnorlax,
	} {
		if !prereqs.Capabilities.Has(capability) {
			t.Fatalf("capabilities missing %q: %v", capability, prereqs.Capabilities)
		}
	}
	fuchsia, ok := Place("fuchsia city")
	if !ok {
		t.Fatal("fuchsia city place missing")
	}

	result, err := world.FindWeightedRoutePlanAtDestinationWithCapabilities(
		g, cinnabarIslandMap, fuchsia.Map, 11, 11, -1, -1, nil, prereqs, redGlobalRouteCostPolicy(prereqs),
	)
	if !errors.Is(err, world.ErrRouteReplanRequired) {
		t.Fatalf("weighted Cinnabar -> Fuchsia: %v", err)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("weighted frontier has %d legs (want one Surf shore): %+v", len(result.Steps), result.Steps)
	}
	step := result.Steps[0]
	if step.Edge.To != semanticRoute21Map {
		t.Fatalf("weighted frontier went %#04x->%#04x, want Route 21 %#04x (not Route 20/Seafoam)",
			step.Edge.From, step.Edge.To, semanticRoute21Map)
	}
	if step.Transition == nil || step.Transition.ID != "red:route21_surf" {
		t.Fatalf("weighted frontier transition = %+v, want red:route21_surf", step.Transition)
	}
}
