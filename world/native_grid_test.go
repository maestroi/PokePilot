package world

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func nativeCutTestGrid(t *testing.T, walkable, cuttable []bool) *NativeGrid {
	t.Helper()
	spec := worldmodel.NativeGridSpec{
		MapID:         0x032c,
		Width:         len(walkable),
		Height:        1,
		Walkable:      walkable,
		CollisionTile: make([]uint8, len(walkable)),
		Cuttable:      cuttable,
	}
	grid, err := NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	return grid
}

func TestFindNativeCutApproachBridgesDisconnectedCorridor(t *testing.T) {
	grid := nativeCutTestGrid(t,
		[]bool{true, true, false, true, true},
		[]bool{false, false, true, false, false},
	)

	plan, err := FindNativeCutApproach(grid, 0, 0, 4, 0, nil)
	if err != nil {
		t.Fatalf("FindNativeCutApproach: %v", err)
	}
	if len(plan.Approach) != 1 || plan.Approach[0] != (NativeStep{DX: 1}) {
		t.Fatalf("approach=%+v, want one step right to stand at x=1", plan.Approach)
	}
	if plan.Cut != (NativeStep{DX: 1}) || plan.TreeX != 2 || plan.TreeY != 0 {
		t.Fatalf("cut plan=%+v, want tree x=2 faced right", plan)
	}
}

func TestFindNativeCutApproachDoesNotAssumeTwoTreesRemoved(t *testing.T) {
	grid := nativeCutTestGrid(t,
		[]bool{true, false, true, false, true},
		[]bool{false, true, false, true, false},
	)

	if _, err := FindNativeCutApproach(grid, 0, 0, 4, 0, nil); !errors.Is(err, ErrNoPath) {
		t.Fatalf("two-tree corridor err=%v, want ErrNoPath for one-action planner", err)
	}
}

func TestFindNativeCutApproachRejectsOccupiedTree(t *testing.T) {
	grid := nativeCutTestGrid(t,
		[]bool{true, true, false, true},
		[]bool{false, false, true, false},
	)
	occupied := map[[2]int]bool{{2, 0}: true}
	if _, err := FindNativeCutApproach(grid, 0, 0, 3, 0, occupied); !errors.Is(err, ErrNoPath) {
		t.Fatalf("occupied tree err=%v, want ErrNoPath", err)
	}
}
