package skill

import (
	"errors"
	"testing"

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

	g, err := world.BuildGraph(romData)
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
