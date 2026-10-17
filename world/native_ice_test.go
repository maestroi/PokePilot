package world

import (
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func TestNativeIceSlidePlansWholeForcedLanding(t *testing.T) {
	spec := worldmodel.NativeGridSpec{
		MapID: 0x0101, Width: 7, Height: 1,
		Walkable: []bool{true, true, true, true, true, true, true},
		CollisionTile: []uint8{0, 0x23, 0x23, 0x23, 0, 0, 0},
		SlideCollisions: map[uint8]bool{0x23: true},
	}
	grid, err := NativeGridFromSpec(spec)
	if err != nil { t.Fatal(err) }
	move, ok := grid.Movement(0, 0, NativeStep{DX: 1}, nil)
	if !ok || move != (NativeStep{DX: 4}) {
		t.Fatalf("ice landing from (0,0) = %+v, ok=%v, want +4", move, ok)
	}
	path, err := FindNativePath(grid, 0, 0, 4, 0, nil)
	if err != nil || len(path) != 1 || path[0] != (NativeStep{DX: 4}) {
		t.Fatalf("ice BFS = %+v err=%v", path, err)
	}
	// Turning partway through a slide is impossible even though those
	// intermediate collision tiles are technically walkable.
	if _, err := FindNativePath(grid, 0, 0, 2, 0, nil); err == nil {
		t.Fatal("native BFS illegally stopped midway through an ice slide")
	}
	// A non-Ice-Path cartridge does not acquire Gen2 slide behavior.
	spec.SlideCollisions = nil
	plain, err := NativeGridFromSpec(spec)
	if err != nil { t.Fatal(err) }
	if move, ok := plain.Movement(0, 0, NativeStep{DX: 1}, nil); !ok || move.DX != 1 {
		t.Fatalf("plain grid inherited slide: %+v", move)
	}
}

func TestNativeIceSlideStopsAtActualObjectOrWall(t *testing.T) {
	spec := worldmodel.NativeGridSpec{
		MapID: 0x0102, Width: 6, Height: 1,
		Walkable: []bool{true, true, true, true, true, false},
		CollisionTile: []uint8{0, 0x23, 0x23, 0x23, 0x23, 7},
		SlideCollisions: map[uint8]bool{0x23: true},
	}
	grid, err := NativeGridFromSpec(spec)
	if err != nil { t.Fatal(err) }
	if move, ok := grid.Movement(0, 0, NativeStep{DX: 1}, nil); !ok || move.DX != 4 {
		t.Fatalf("wall did not stop sliding: %+v ok=%v", move, ok)
	}
	block := map[[2]int]bool{{3, 0}:true}
	if move, ok := grid.Movement(0, 0, NativeStep{DX: 1}, block); !ok || move.DX != 2 {
		t.Fatalf("occupied ice stop = %+v ok=%v", move, ok)
	}
}
