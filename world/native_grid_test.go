package world

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func nativeCutTestGrid(t *testing.T, walkable, cuttable []bool) *NativeGrid {
	t.Helper()
	obstacles := make([]worldmodel.NativeObstacle, len(cuttable))
	for i, cut := range cuttable {
		if cut {
			obstacles[i] = worldmodel.ObstacleCutTree
		}
	}
	spec := worldmodel.NativeGridSpec{
		MapID:         0x032c,
		Width:         len(walkable),
		Height:        1,
		Walkable:      walkable,
		CollisionTile: make([]uint8, len(walkable)),
		Obstacles:     obstacles,
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

	plan, err := findNativeCutApproach(grid, 0, 0, 4, 0, nil)
	if err != nil {
		t.Fatalf("FindNativeCutApproach: %v", err)
	}
	if len(plan.Approach) != 1 || plan.Approach[0] != (NativeStep{DX: 1}) {
		t.Fatalf("approach=%+v, want one step right to stand at x=1", plan.Approach)
	}
	if plan.Clear != (NativeStep{DX: 1}) || plan.X != 2 || plan.Y != 0 {
		t.Fatalf("cut plan=%+v, want tree x=2 faced right", plan)
	}
}

func TestFindNativeCutApproachDoesNotAssumeTwoTreesRemoved(t *testing.T) {
	grid := nativeCutTestGrid(t,
		[]bool{true, false, true, false, true},
		[]bool{false, true, false, true, false},
	)

	if _, err := findNativeCutApproach(grid, 0, 0, 4, 0, nil); !errors.Is(err, ErrNoPath) {
		t.Fatalf("two-tree corridor err=%v, want ErrNoPath for one-action planner", err)
	}
}

func TestFindNativeCutApproachRejectsOccupiedTree(t *testing.T) {
	grid := nativeCutTestGrid(t,
		[]bool{true, true, false, true},
		[]bool{false, false, true, false},
	)
	occupied := map[[2]int]bool{{2, 0}: true}
	if _, err := findNativeCutApproach(grid, 0, 0, 3, 0, occupied); !errors.Is(err, ErrNoPath) {
		t.Fatalf("occupied tree err=%v, want ErrNoPath", err)
	}
}

// TestFindNativePathLeavesSolidWarpStart pins the Sprout Tower stairs contract:
// Gen II stair/door landings are solid collision tiles the player is already
// standing on. FindNativePath must leave them the same way FindPath does, or
// the router proves every sibling stair unreachable and loops the entrance
// (farm run-x330xhsmcfod).
func TestFindNativePathLeavesSolidWarpStart(t *testing.T) {
	spec := worldmodel.NativeGridSpec{
		MapID:         0x0301,
		Width:         5,
		Height:        1,
		Walkable:      []bool{false, true, true, true, true},
		CollisionTile: make([]uint8, 5),
	}
	grid, err := NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	path, err := FindNativePath(grid, 0, 0, 4, 0, nil)
	if err != nil {
		t.Fatalf("FindNativePath from solid warp start: %v", err)
	}
	if len(path) != 4 {
		t.Fatalf("path=%+v, want 4 steps off the solid landing", path)
	}
	if grid.Walkable(0, 0) {
		t.Fatal("solid start tile became walkable")
	}
}

func findNativeCutApproach(g *NativeGrid, sx, sy, tx, ty int, occupied map[[2]int]bool) (NativeObstacleApproach, error) {
	return FindNativeObstacleApproach(g, sx, sy, tx, ty, occupied, func(k worldmodel.NativeObstacle) bool { return k == worldmodel.ObstacleCutTree })
}

func TestFindNativeObstacleApproachSmashesOccupyingRockOnlyWhenUsable(t *testing.T) {
	grid := nativeCutTestGrid(t, []bool{true, true, true, true, true}, nil)
	grid.SetObjectObstacle(2, 0, worldmodel.ObstacleSmashRock)
	rock := map[[2]int]bool{{2, 0}: true}
	if _, err := FindNativePath(grid, 0, 0, 4, 0, rock); !errors.Is(err, ErrNoPath) {
		t.Fatalf("plain path through a rock = %v, want ErrNoPath", err)
	}
	smash := func(k worldmodel.NativeObstacle) bool { return k == worldmodel.ObstacleSmashRock }
	plan, err := FindNativeObstacleApproach(grid, 0, 0, 4, 0, rock, smash)
	if err != nil {
		t.Fatalf("FindNativeObstacleApproach: %v", err)
	}
	if plan.Kind != worldmodel.ObstacleSmashRock || plan.X != 2 || plan.Y != 0 || len(plan.Approach) != 1 {
		t.Fatalf("plan = %+v, want rock at (2,0) after one approach step", plan)
	}
	cutOnly := func(k worldmodel.NativeObstacle) bool { return k == worldmodel.ObstacleCutTree }
	if _, err := FindNativeObstacleApproach(grid, 0, 0, 4, 0, rock, cutOnly); !errors.Is(err, ErrNoPath) {
		t.Fatalf("approach without Rock Smash = %v, want ErrNoPath", err)
	}
}
