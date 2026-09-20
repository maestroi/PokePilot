package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// TestSilphCo5FCardKeyTopologyUsesWarpPortalsNotCorridors locks the shared
// routing invariant behind farm run-29v9xyjvb2vlx1kkxbzqocuax4:
//
// On Silph Co 5F, the MovementStay rocket at (8,16) cuts the stair/elevator
// side from the Card Key approach. The only remaining join is through warp
// tiles. Those tiles are portals (Traverse edges), not walkable corridors —
// GoTo's local walk already refuses them via warpAvoidance. Component
// topology must agree: after present stay homes and warp tiles are removed
// from the walkable grid, the two pockets are different components and
// FindRoutePlan leaves through 9F instead of returning an empty same-map route.
func TestSilphCo5FCardKeyTopologyUsesWarpPortalsNotCorridors(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	h, err := rom.ParseMap(romData, silphCo5FMap)
	if err != nil {
		t.Fatalf("ParseMap(Silph Co 5F): %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(Silph Co 5F): %v", err)
	}

	const (
		stairX, stairY       = 26, 1
		approachX, approachY = 20, 16
		rocketX, rocketY     = 8, 16
	)

	present := presentStationaryObjectBlockers(h, nil)
	if !present[[2]int{rocketX, rocketY}] {
		t.Fatal("Silph Co 5F rocket is not a present MovementStay home")
	}

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	overlaid, err := overlayObservedMapTopology(g, grid, h, present)
	if err != nil {
		t.Fatalf("overlayObservedMapTopology: %v", err)
	}

	comps := world.Components(grid)
	if comps[stairY][stairX] == 0 || comps[approachY][approachX] == 0 {
		t.Fatalf("stair (%d,%d) or approach (%d,%d) is not in a walkable component", stairX, stairY, approachX, approachY)
	}
	if comps[stairY][stairX] == comps[approachY][approachX] {
		t.Fatalf("stair and Card Key approach share component %d after present+warp overlay; warp tiles are still acting as corridors", comps[stairY][stairX])
	}

	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		overlaid, silphCo5FMap, silphCo5FMap,
		stairX, stairY, approachX, approachY,
		nil, world.RoutePrerequisites{},
	)
	if err != nil {
		t.Fatalf("FindRoutePlanAtDestination: %v", err)
	}
	if len(route) == 0 {
		t.Fatal("FindRoutePlanAtDestination returned an empty same-map route; expected a leave/re-enter plan through another Silph floor")
	}
	if route[0].Edge.To == silphCo5FMap {
		t.Fatalf("first leg stays on 5F (%+v); want a portal to another floor", route[0].Edge)
	}
}
