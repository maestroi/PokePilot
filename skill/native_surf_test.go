package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

func nativeSurfTestGrid(t *testing.T, walkable []bool, mode worldmodel.TraversalMode) *world.NativeGrid {
	t.Helper()
	grid, err := world.NativeGridFromSpec(worldmodel.NativeGridSpec{
		MapID:         0x1804,
		Width:         len(walkable),
		Height:        1,
		Walkable:      walkable,
		CollisionTile: make([]byte, len(walkable)),
		Traversal:     mode,
	})
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	return grid
}

func TestNativeSurfApproachWalksToShoreBeforeUsingMove(t *testing.T) {
	// On Route 40 the player must walk to shore, face water and use Surf
	// before native routing may enter a water-only cell.
	land := nativeSurfTestGrid(t, []bool{true, true, false, false, true}, worldmodel.TraversalLand)
	water := nativeSurfTestGrid(t, []bool{true, true, true, true, true}, worldmodel.TraversalWater)

	plan, ok := planNativeSurfApproach(land, water, 0, 0, [][2]int{{4, 0}}, nil)
	if !ok {
		t.Fatal("water crossing was not detected")
	}
	if len(plan.approach) != 1 || plan.approach[0] != (world.NativeStep{DX: 1}) {
		t.Fatalf("land-only approach = %+v, want one step right", plan.approach)
	}
	if plan.shore.X != 1 || plan.shore.Y != 0 || plan.water.X != 2 || plan.water.Y != 0 {
		t.Fatalf("shore=%+v water=%+v, want (1,0) -> (2,0)", plan.shore, plan.water)
	}
	// The reverse trip must start Surf from the eastern shore rather than
	// walking into water first or trying to surf from the opposite bank.
	reverse, ok := planNativeSurfApproach(land, water, 4, 0, [][2]int{{0, 0}}, nil)
	if !ok || reverse.shore.X != 4 || reverse.water.X != 3 || len(reverse.approach) != 0 {
		t.Fatalf("reverse crossing = %+v ok=%v, want east shore -> west water", reverse, ok)
	}
}

func TestNativeSurfApproachDoesNotSurfWhenLandCanReachTarget(t *testing.T) {
	all := []bool{true, true, true, true}
	land := nativeSurfTestGrid(t, all, worldmodel.TraversalLand)
	water := nativeSurfTestGrid(t, all, worldmodel.TraversalWater)
	if plan, ok := planNativeSurfApproach(land, water, 0, 0, [][2]int{{3, 0}}, nil); ok {
		t.Fatalf("ordinary walk was incorrectly classified as Surf: %+v", plan)
	}
}

func TestNativeSurfApproachRequiresRealWaterRoute(t *testing.T) {
	land := nativeSurfTestGrid(t, []bool{true, true, false, false, true}, worldmodel.TraversalLand)
	water := nativeSurfTestGrid(t, []bool{true, true, true, false, true}, worldmodel.TraversalWater)
	if plan, ok := planNativeSurfApproach(land, water, 0, 0, [][2]int{{4, 0}}, nil); ok {
		t.Fatalf("blocked water corridor falsely yielded Surf approach: %+v", plan)
	}
}

func TestNativeSurfApproachDoesNotWalkThroughSpriteOnShore(t *testing.T) {
	land := nativeSurfTestGrid(t, []bool{true, true, false, false, true}, worldmodel.TraversalLand)
	water := nativeSurfTestGrid(t, []bool{true, true, true, true, true}, worldmodel.TraversalWater)
	occupied := map[[2]int]bool{{1, 0}: true}
	if plan, ok := planNativeSurfApproach(land, water, 0, 0, [][2]int{{4, 0}}, occupied); ok {
		t.Fatalf("blocked shoreline falsely yielded Surf approach: %+v", plan)
	}
}

func TestNativeSurfApproachSkipsUnreachableTargets(t *testing.T) {
	land := nativeSurfTestGrid(t, []bool{true, false, false, true}, worldmodel.TraversalLand)
	water := nativeSurfTestGrid(t, []bool{true, true, true, true}, worldmodel.TraversalWater)
	plan, ok := planNativeSurfApproach(land, water, 0, 0, [][2]int{{8, 0}, {3, 0}}, nil)
	if !ok || plan.shore.X != 0 || plan.water.X != 1 {
		t.Fatalf("alternate reachable water target = %+v ok=%v", plan, ok)
	}
}

func TestNativeSurfApproachRequiresMatchingLiveMap(t *testing.T) {
	land := nativeSurfTestGrid(t, []bool{true, false}, worldmodel.TraversalLand)
	water := nativeSurfTestGrid(t, []bool{true, true}, worldmodel.TraversalWater)
	water.MapID++ // Never plan a shoreline from another native map.
	if _, ok := planNativeSurfApproach(land, water, 0, 0, [][2]int{{1, 0}}, nil); ok {
		t.Fatal("mismatched map identities were treated as the same shore")
	}
}
