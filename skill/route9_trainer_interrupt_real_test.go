package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

// TestRoute9CutApproachTrainerInterruptRealROM pins
// run-12vowvyawgx0b3jl0srufdx8tq rounds 34/36: Traverse's Cut-aware approach
// to Route 9's east band cut the tree, then Route 9's bug catcher at (40,8)
// spotted Red. The interruption was swallowed and reported as an exhausted
// connection band, so GoTo banned Route 9 -> Route 10 for the journey and
// looped back through Vermilion/Diglett's Cave until the navigation guard fired.
//
// POKEPILOT_ROUTE9_INTERRUPT_STATE is that run's
// failure-frame-0000357174-progress-saffron-gate-open.state (Route 6, Cut
// usable, Route 9 trainers unbeaten). It was saved from the farm's
// Mewtwo-starter ROM; set POKEPILOT_FARM_ROM to it.
func TestRoute9CutApproachTrainerInterruptRealROM(t *testing.T) {
	path, farmPath := os.Getenv("POKEPILOT_ROUTE9_INTERRUPT_STATE"), os.Getenv("POKEPILOT_FARM_ROM")
	if path == "" || farmPath == "" {
		t.Skip("set POKEPILOT_ROUTE9_INTERRUPT_STATE and POKEPILOT_FARM_ROM")
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

	dest, ok := Place("route 10")
	if !ok {
		t.Fatal("route 10 place missing")
	}
	if _, err := Travel(m, base, dest, StatAwareMove(base), 20); err != nil {
		t.Fatalf("Travel Route 10 from Route 6: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != route10Map {
		t.Fatalf("after Travel map=%#02x, want Route 10 %#02x", got, route10Map)
	}
}
