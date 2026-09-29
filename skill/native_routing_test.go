package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"

	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

func nativeTestGrid(t *testing.T, width, height int, walkable []bool) *world.NativeGrid {
	t.Helper()
	spec := worldmodel.NativeGridSpec{
		MapID:         0x1804,
		Width:         width,
		Height:        height,
		Walkable:      walkable,
		CollisionTile: make([]uint8, width*height),
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	return grid
}

func TestNativeAdjacentApproachStopsBesideWarp(t *testing.T) {
	grid := nativeTestGrid(t, 5, 5, []bool{
		true, true, true, true, true,
		true, true, true, true, true,
		true, true, true, true, true,
		true, true, true, true, true,
		true, true, true, true, true,
	})
	blocked := map[[2]int]bool{{2, 2}: true}
	path, push, err := nativeAdjacentApproach(grid, 0, 2, 2, 2, blocked)
	if err != nil {
		t.Fatalf("nativeAdjacentApproach: %v", err)
	}
	if len(path) != 1 || path[0] != (world.NativeStep{DX: 1}) {
		t.Fatalf("path = %+v, want one step right to (1,2)", path)
	}
	if push != (world.NativeStep{DX: 1}) {
		t.Fatalf("push = %+v, want right", push)
	}
}

type nativeConnectionTestProvider map[uint16]worldmodel.NativeMapHeader

func (p nativeConnectionTestProvider) MapIDs() []uint16 {
	out := make([]uint16, 0, len(p))
	for id := range p {
		out = append(out, id)
	}
	return out
}

func (p nativeConnectionTestProvider) ParseMap(id uint16) (worldmodel.NativeMapHeader, error) {
	return p[id], nil
}

func TestNativeConnectionApproachHonorsOffsetBounds(t *testing.T) {
	grid := nativeTestGrid(t, 6, 4, []bool{
		true, true, true, true, true, true,
		true, true, true, true, true, true,
		true, true, true, true, true, true,
		true, true, true, true, true, true,
	})
	provider := nativeConnectionTestProvider{
		0x1804: {ID: 0x1804, WidthBlocks: 3, HeightBlocks: 2},
		0x1803: {ID: 0x1803, WidthBlocks: 2, HeightBlocks: 2},
	}
	edge := world.NativeEdge{
		Kind:   world.EdgeConnection,
		From:   0x1804,
		To:     0x1803,
		Dir:    0,
		Offset: -2,
	}
	path, push, err := nativeConnectionApproach(provider, grid, edge, 5, 3, nil)
	if err != nil {
		t.Fatalf("nativeConnectionApproach: %v", err)
	}
	if push != (world.NativeStep{DY: -1}) {
		t.Fatalf("push = %+v, want up", push)
	}
	// Destination width is four tiles. With offset -2 only source x=2..5
	// are valid; from (5,3), x=5 is the nearest valid north-edge crossing.
	if len(path) != 3 {
		t.Fatalf("path length = %d, want 3 to north edge at x=5: %+v", len(path), path)
	}
}

func TestNativeRuntimeBlockersAvoidWarpsAndObjects(t *testing.T) {
	live := game.LiveTopologyState{
		LiveObjects: []game.LiveMapObject{{Slot: 1, X: 3, Y: 4}},
	}
	header := worldmodel.NativeMapHeader{
		Warps: []worldmodel.NativeWarp{
			{X: 1, Y: 2},
			{X: 5, Y: 6},
		},
	}
	allow := [2]int{1, 2}
	blocked := nativeRuntimeBlockers(live, header, &allow)
	if blocked[[2]int{1, 2}] {
		t.Fatal("allowed warp was blocked")
	}
	if !blocked[[2]int{5, 6}] {
		t.Fatal("other warp was not blocked")
	}
	if !blocked[[2]int{3, 4}] {
		t.Fatal("live object was not blocked")
	}
}


func TestNativeCutPathMarksTreeEntry(t *testing.T) {
	spec := worldmodel.NativeGridSpec{
		MapID: 0x032c,
		Width: 5,
		Height: 1,
		Walkable: []bool{true, true, false, true, true},
		CollisionTile: []uint8{0, 0, 0x12, 0, 0},
		Cuttable: []bool{false, false, true, false, false},
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	if _, err := world.FindNativePath(grid, 0, 0, 4, 0, nil); err == nil {
		t.Fatal("ordinary path unexpectedly crossed Cut tree")
	}
	path, err := world.FindNativePathWithCut(grid, 0, 0, 4, 0, nil, true)
	if err != nil {
		t.Fatalf("FindNativePathWithCut: %v", err)
	}
	if len(path) != 4 || path[0].Cut || path[1].Cut != true || path[2].Cut || path[3].Cut {
		t.Fatalf("cut path=%+v, want only entry into x=2 marked Cut", path)
	}
}

func TestNativeCutPathStaysClosedWithoutCapability(t *testing.T) {
	spec := worldmodel.NativeGridSpec{
		MapID: 0x032c,
		Width: 3,
		Height: 1,
		Walkable: []bool{true, false, true},
		CollisionTile: []uint8{0, 0x1a, 0},
		Cuttable: []bool{false, true, false},
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	if _, err := world.FindNativePathWithCut(grid, 0, 0, 2, 0, nil, false); err == nil {
		t.Fatal("Cut-sealed path opened without Cut capability")
	}
}
