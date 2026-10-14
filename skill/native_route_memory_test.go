package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// A map entered by connection has no warp entry. Its sealed-edge proof must
// still cover that entry, or the router re-picks the same dead seam until the
// transition budget runs out (Gold Route 38 east seam -> Ecruteak).
func TestNativeUnreachableProofCoversConnectionEntry(t *testing.T) {
	grid, err := world.NativeGridFromSpec(worldmodel.NativeGridSpec{
		MapID: 0x010c, Width: 3, Height: 1,
		Walkable:      []bool{true, true, true},
		CollisionTile: make([]uint8, 3),
	})
	if err != nil {
		t.Fatal(err)
	}
	header := worldmodel.NativeMapHeader{Warps: []worldmodel.NativeWarp{{X: 2, Y: 0}}}
	edge := world.NativeEdge{Kind: world.EdgeConnection, From: 0x010c, To: 0x0409}
	mem := NewNativeRouteMemory()
	mem.entry = world.NativeEntryUnknown

	nativeMarkComponentUnreachable(mem, grid, header, 0x010c, 0, 0, edge)

	for _, entry := range []int{world.NativeEntryUnknown, 0} {
		if !mem.unreachable[world.NativeUnreachable{Map: 0x010c, Entry: entry, Edge: edge}] {
			t.Fatalf("sealed edge not recorded for entry %d: %v", entry, mem.unreachable)
		}
	}
}

// Gold's Route 38 gate warps are COLL_WARP_CARPET_RIGHT: stepping onto the
// tile from a neighbour never fires them. The approach stands on the carpet
// and pushes toward its direction.
func TestNativeWarpApproachStandsOnEdgeCarpet(t *testing.T) {
	grid, err := world.NativeGridFromSpec(worldmodel.NativeGridSpec{
		MapID: 0x010c, Width: 3, Height: 1,
		Walkable:      []bool{true, true, true},
		CollisionTile: []uint8{0x00, 0x00, 0x7e},
		WarpPushes:    map[uint8]worldmodel.NativeDirectionMask{0x7e: worldmodel.NativeBlockRight},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Runtime blockers include every warp tile; the carpet must still be the
	// approach's destination.
	path, push, err := nativeWarpApproach(grid, 0, 0, 2, 0, map[[2]int]bool{{2, 0}: true})
	if err != nil || len(path) != 2 || push != (world.NativeStep{DX: 1}) {
		t.Fatalf("carpet approach = %v push=%+v err=%v, want two steps onto the carpet then push right", path, push, err)
	}
	if got := nativeWarpTargets(grid, 2, 0, map[[2]int]bool{{2, 0}: true}); len(got) != 1 || got[0] != [2]int{2, 0} {
		t.Fatalf("carpet targets = %v, want the carpet tile itself", got)
	}
}

// An edge carpet sealed behind a Cut tree must still be a bridge target: the
// carpet is in the runtime blockers like every warp tile. Route 35's Goldenrod
// gate carpets behind the (17,6) tree read as unreachable, stranding the run
// north of it (run-whai2l6l8g1x36h0y0fucmsit / triage efe8f75648bc29a8).
func TestNativeObstacleBridgeTargetsSealedEdgeCarpet(t *testing.T) {
	grid, err := world.NativeGridFromSpec(worldmodel.NativeGridSpec{
		MapID: 0x0a02, Width: 4, Height: 1,
		Walkable:      []bool{true, false, true, true},
		CollisionTile: []uint8{0x00, 0x12, 0x00, 0x70},
		Obstacles:     []worldmodel.NativeObstacle{"", worldmodel.ObstacleCutTree, "", ""},
		WarpPushes:    map[uint8]worldmodel.NativeDirectionMask{0x70: worldmodel.NativeBlockRight},
	})
	if err != nil {
		t.Fatal(err)
	}
	blocked := map[[2]int]bool{{3, 0}: true}
	targets := nativeWarpTargets(grid, 3, 0, blocked)
	if len(targets) != 1 || targets[0] != [2]int{3, 0} {
		t.Fatalf("carpet targets = %v, want the carpet tile itself", targets)
	}
	usable := func(kind worldmodel.NativeObstacle) bool { return kind == worldmodel.ObstacleCutTree }
	plan, err := world.FindNativeObstacleApproach(grid, 0, 0, 3, 0, nativeAllowTarget(blocked, targets[0]), usable)
	if err != nil || plan.X != 1 || plan.Kind != worldmodel.ObstacleCutTree {
		t.Fatalf("bridge plan = %+v err=%v, want Cut at (1,0)", plan, err)
	}
	if !blocked[[2]int{3, 0}] {
		t.Fatal("nativeAllowTarget mutated the caller's blockers")
	}
}
