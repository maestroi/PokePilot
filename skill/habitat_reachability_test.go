package skill_test

import (
	"errors"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
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

// TestCatchHabitatBlockNamesMissingCapabilities pins the execution half of
// #2480: a plain-grass catch whose encounter cells sit behind Surf and the
// badge checks must surface that route blockage, not an untyped "no encounter
// cells" miss. The latter was classified outcome:blocked and exhausted the
// failure-recovery budget (run-3w50i60f631u41je7v36wsudzt).
func TestCatchHabitatBlockNamesMissingCapabilities(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")
	romData, err := os.ReadFile(os.Getenv("POKEMON_RED_ROM"))
	if err != nil {
		t.Fatal(err)
	}
	route1, _ := skill.Place("route 1")
	route23, _ := skill.Place("route 23")
	if err := skill.CatchHabitatBlock(m, romData, route1.Map); err != nil {
		t.Fatalf("route 1 grass: %v", err)
	}
	err = skill.CatchHabitatBlock(m, romData, route23.Map)
	var blocked *world.RouteBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("route 23 grass block = %v, want RouteBlockedError", err)
	}
	missing := map[string]bool{}
	for _, id := range blocked.MissingCapabilities() {
		missing[string(id)] = true
	}
	if !missing["can_surf"] {
		t.Fatalf("missing capabilities = %v, want can_surf", blocked.MissingCapabilities())
	}
}
