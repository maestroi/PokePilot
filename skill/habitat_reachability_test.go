package skill_test

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestGrassHabitatReachableNeedsCapabilities pins #2480/#2482: Route 23 is a
// map-level arrival long before its grass is (the grass sits behind Surf and
// the badge checks), so map reachability must not stand in for a catch
// habitat. Route 1's grass is plain walking from Viridian.
func TestGrassHabitatReachableNeedsCapabilities(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")
	romData, err := os.ReadFile(os.Getenv("POKEMON_RED_ROM"))
	if err != nil {
		t.Fatal(err)
	}
	reachable, err := skill.GrassHabitatReachable(m, romData)
	if err != nil || reachable == nil {
		t.Fatalf("GrassHabitatReachable: nil=%v err=%v", reachable == nil, err)
	}
	route1, _ := skill.Place("route 1")
	route23, _ := skill.Place("route 23")
	if !reachable(route1.Map) {
		t.Errorf("route 1 grass reported unreachable from Viridian")
	}
	if reachable(route23.Map) {
		t.Errorf("route 23 grass reported reachable without Surf or badges")
	}
}
