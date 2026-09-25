package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

func TestRoute13StationaryTrainerMakesRow8FreshReentry(t *testing.T) {
	p := os.Getenv("POKEMON_RED_ROM")
	if p == "" {
		t.Skip("POKEMON_RED_ROM required")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	g, err := world.BuildGraph(data)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	const (
		route13 = uint8(0x18)
		route14 = uint8(0x19)
	)
	h13, err := rom.ParseMap(data, route13)
	if err != nil {
		t.Fatalf("ParseMap Route 13: %v", err)
	}
	grid13, err := world.Build(data, h13)
	if err != nil {
		t.Fatalf("Build Route 13 grid: %v", err)
	}
	trainerSlot := 0
	for i, o := range h13.Objects {
		if o.X == 12 && o.Y == 4 {
			trainerSlot = i + 1
			break
		}
	}
	if trainerSlot == 0 {
		t.Fatal("Route 13 object at (12,4) is missing")
	}
	observed13 := observedStationaryObjectBlockers(h13, []game.LiveMapObject{{Slot: trainerSlot, X: 12, Y: 4}})
	if !observed13[[2]int{12, 4}] {
		t.Fatal("Route 13 object at (12,4) is no longer a visible MovementStay blocker")
	}
	g, err = overlayObservedMapTopology(g, grid13, h13, observed13)
	if err != nil {
		t.Fatalf("overlay Route 13: %v", err)
	}

	// GoTo observes Route 14 after leaving Route 13. Updating Route 14 must
	// retain Route 13's stationary-trainer split for the return leg.
	h14, err := rom.ParseMap(data, route14)
	if err != nil {
		t.Fatalf("ParseMap Route 14: %v", err)
	}
	grid14, err := world.Build(data, h14)
	if err != nil {
		t.Fatalf("Build Route 14 grid: %v", err)
	}
	g, err = overlayObservedMapTopology(g, grid14, h14, nil)
	if err != nil {
		t.Fatalf("overlay Route 14: %v", err)
	}

	var row4, row8 *world.Edge
	for i := range g.Edges[route14] {
		e := &g.Edges[route14][i]
		if e.To != route13 || e.Kind != world.EdgeConnection {
			continue
		}
		start, end, ok := world.ConnectionBand(*e)
		if !ok {
			continue
		}
		if start <= 4 && 4 <= end {
			row4 = e
		}
		if start <= 8 && 8 <= end {
			row8 = e
		}
	}
	if row4 == nil || row8 == nil {
		t.Fatalf("missing Route 14 -> Route 13 bands: row4=%v row8=%v", row4, row8)
	}

	if same, known := g.EdgeEntrySharesComponentWith(*row4, 11, 4); !known || !same {
		t.Fatalf("row-4 entry relative to (11,4) = same %t, known %t; want same", same, known)
	}
	if same, known := g.EdgeEntrySharesComponentWith(*row8, 11, 4); !known || same {
		t.Fatalf("row-8 entry relative to (11,4) = same %t, known %t; want fresh component", same, known)
	}
}

// Travel resumes GoTo after dialogue/battle with shared navigationMemory.
// Without carrying the observed Route 13 topology into that next call,
// EdgeEntrySharesComponentWith collapses back to the ROM-only one-component
// map and visitedMaps bans every Route 14 -> Route 13 edge — including the
// fresh row-8 re-entry that is the only escape from the west trainer pocket
// (run-4h4isxsvaskt1c7mslvxzsrr6).
func TestNavigationMemoryRetainsRoute13TopologyAcrossGoToCalls(t *testing.T) {
	p := os.Getenv("POKEMON_RED_ROM")
	if p == "" {
		t.Skip("POKEMON_RED_ROM required")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	base, err := world.BuildGraph(data)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	const (
		route13 = uint8(0x18)
		route14 = uint8(0x19)
		route15 = uint8(0x1a)
	)
	h13, err := rom.ParseMap(data, route13)
	if err != nil {
		t.Fatalf("ParseMap Route 13: %v", err)
	}
	grid13, err := world.Build(data, h13)
	if err != nil {
		t.Fatalf("Build Route 13 grid: %v", err)
	}
	observed13 := map[[2]int]bool{{12, 4}: true}
	observed, err := overlayObservedMapTopology(base, grid13, h13, observed13)
	if err != nil {
		t.Fatalf("overlay Route 13: %v", err)
	}
	h14, err := rom.ParseMap(data, route14)
	if err != nil {
		t.Fatalf("ParseMap Route 14: %v", err)
	}
	grid14, err := world.Build(data, h14)
	if err != nil {
		t.Fatalf("Build Route 14 grid: %v", err)
	}
	observed, err = overlayObservedMapTopology(observed, grid14, h14, nil)
	if err != nil {
		t.Fatalf("overlay Route 14: %v", err)
	}

	var row8 world.Edge
	foundRow8 := false
	for _, e := range observed.Edges[route14] {
		if e.To != route13 || e.Kind != world.EdgeConnection {
			continue
		}
		start, end, ok := world.ConnectionBand(e)
		if ok && start <= 8 && 8 <= end {
			row8 = e
			foundRow8 = true
			break
		}
	}
	if !foundRow8 {
		t.Fatal("missing Route 14 -> Route 13 row-8 band")
	}

	visitedMaps := map[uint8]bool{route13: true}
	visitedPositions := map[uint8][]navigationState{
		route13: {{Map: route13, X: 11, Y: 4}},
	}

	// Fresh ROM graph (what a second GoTo used to start from): row-8 looks
	// same-component and is banned.
	if !edgeEntersVisitedRegion(base, row8, visitedMaps, visitedPositions) {
		t.Fatal("ROM-only graph must ban row-8 when Route 13 was visited at (11,4)")
	}
	freshFilter := graphWithoutEdgesInto(base, visitedMaps, visitedPositions)
	freshRoute, freshErr := world.FindRoutePlanAtDestinationWithCapabilities(
		freshFilter, route14, celadonMartRoofMap, 19, 4, int(vendingStandX), int(vendingStandY), nil, world.RoutePrerequisites{},
	)
	if freshErr != nil {
		t.Fatalf("ROM-only plan from Route 14: %v", freshErr)
	}
	if freshRoute[0].Edge.To != route15 {
		t.Fatalf("ROM-only first hop = %#02x, want Route 15 detour %#02x", freshRoute[0].Edge.To, route15)
	}

	// Shared journey memory keeps the observed split: row-8 stays eligible
	// and is the first hop toward Celadon.
	nav := newNavigationMemory()
	nav.routeGraph = observed
	nav.visitedMaps = visitedMaps
	nav.visitedPositions = visitedPositions
	if edgeEntersVisitedRegion(nav.routeGraph, row8, nav.visitedMaps, nav.visitedPositions) {
		t.Fatal("remembered topology must treat row-8 as a fresh-component re-entry")
	}
	keptFilter := graphWithoutEdgesInto(nav.routeGraph, nav.visitedMaps, nav.visitedPositions)
	keptRoute, keptErr := world.FindRoutePlanAtDestinationWithCapabilities(
		keptFilter, route14, celadonMartRoofMap, 19, 4, int(vendingStandX), int(vendingStandY), nil, world.RoutePrerequisites{},
	)
	if keptErr != nil {
		t.Fatalf("remembered-topology plan from Route 14: %v", keptErr)
	}
	if keptRoute[0].Edge.To != route13 {
		t.Fatalf("remembered-topology first hop = %#02x, want Route 13 re-entry %#02x", keptRoute[0].Edge.To, route13)
	}
}
