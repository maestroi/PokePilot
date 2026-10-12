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
