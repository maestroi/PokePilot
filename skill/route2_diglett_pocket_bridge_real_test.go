package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

// TestRoute2DiglettPocketBridgeRealROM pins issue #2006: standing in Route 2's
// Diglett's Cave pocket (12,10), every route toward the League crosses a
// semantic gate, so the policy planner only ever returns a boundary prefix.
// The field-path bridge rejected that prefix and GoTo died on "no route"
// (run-46xvnrqmqwcwvjnfqu5u03nj, attempts 24-32).
//
// POKEPILOT_ROUTE2_POCKET_STATE is that run's
// failure-frame-0001725635-progress-indigo-plateau-ready.state. It was saved
// from the farm's Mewtwo-starter ROM; set POKEPILOT_FARM_ROM to it.
func TestRoute2DiglettPocketBridgeRealROM(t *testing.T) {
	path, farmPath := os.Getenv("POKEPILOT_ROUTE2_POCKET_STATE"), os.Getenv("POKEPILOT_FARM_ROM")
	if path == "" || farmPath == "" {
		t.Skip("set POKEPILOT_ROUTE2_POCKET_STATE and POKEPILOT_FARM_ROM")
	}
	m := openEmuCGB(t)
	base := append([]byte(nil), m.ROM()...)
	farm, err := os.ReadFile(farmPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.LoadDerivedROM(base, farm, "farm"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.LoadState(b); err != nil {
		t.Fatal(err)
	}
	defer WithTravelCostPolicy(m, TravelCostFastest)()
	if got := m.Peek8(sym.CurMap); got != semanticRoute2Map {
		t.Fatalf("prepared state on map %#02x, want Route 2 %#02x", got, semanticRoute2Map)
	}

	dest := ExactDestination(route23Map, route23VictoryRoadWarpX, route23VictoryRoadApproachY)
	if _, err := Travel(m, base, dest, StatAwareMove(base), 20); err != nil {
		t.Fatalf("Travel Route 23 from Route 2 Diglett pocket: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != route23Map {
		t.Fatalf("after Travel map=%#02x, want Route 23 %#02x", got, route23Map)
	}
}
