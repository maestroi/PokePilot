package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// TestSilphCo5FSealedCardKeyCorridorNeedsWarpDetour locks the topology that
// made run-77gc6gouf4ph1udc0lfs70evu die on no_path: with every stationary
// home and unrelated warp treated as solid (walkWithinMap's preferred
// liveBlockers + warpAvoidance), the Card Key approach is on a different
// walkable component from the elevator landing, and the component router must
// leave via Silph Co 9F rather than claim a same-map walk.
func TestSilphCo5FSealedCardKeyCorridorNeedsWarpDetour(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	h, err := rom.ParseMap(romData, silphCo5FMap)
	if err != nil {
		t.Fatal(err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatal(err)
	}
	const startX, startY = 28, 3
	const destX, destY = 20, 16
	sealed := warpAvoidance(h, startX, startY, stationaryObjectBlockers(h))
	if _, err := world.FindPath(grid, startX, startY, destX, destY, sealed); err == nil {
		t.Fatal("same-map walk through sealed Silph Co 5F must be impossible")
	}

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatal(err)
	}
	sealedGrid, err := world.Build(romData, h)
	if err != nil {
		t.Fatal(err)
	}
	for at := range sealed {
		sealedGrid.Set(at[0], at[1], false)
	}
	g2, err := g.WithMapGrid(silphCo5FMap, sealedGrid)
	if err != nil {
		t.Fatal(err)
	}
	route, err := world.FindRouteAtDestination(g2, silphCo5FMap, silphCo5FMap, startX, startY, destX, destY, nil)
	if err != nil {
		t.Fatalf("sealed component route: %v", err)
	}
	if len(route) < 2 {
		t.Fatalf("want leave-and-reenter route, got %d legs: %+v", len(route), route)
	}
	if route[0].From != silphCo5FMap || route[0].To == silphCo5FMap {
		t.Fatalf("first leg should leave 5F, got %+v", route[0])
	}
	if route[len(route)-1].To != silphCo5FMap {
		t.Fatalf("last leg should re-enter 5F, got %+v", route[len(route)-1])
	}
}
