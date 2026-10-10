package world

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func nativeStrengthTestGrid(t *testing.T) *NativeGrid {
	t.Helper()
	// Only the center corridor leads to the goal. The short upper branch
	// lets the player stand above the boulder and push it into a lower alcove.
	spec := worldmodel.NativeGridSpec{
		MapID: 0x1804, Width: 5, Height: 3,
		Walkable: []bool{
			true, true, true, false, false,
			true, true, true, true, true,
			false, false, true, false, false,
		},
		CollisionTile: make([]byte, 15),
	}
	grid, err := NativeGridFromSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	return grid
}

func TestNativeStrengthFindsDeterministicBoulderBridge(t *testing.T) {
	grid := nativeStrengthTestGrid(t)
	start := Point{X: 0, Y: 1}
	boulder := Movable{ID: 4, Pos: Point{X: 2, Y: 1}}
	push, err := FindNativeStrengthPush(grid, start, []Movable{boulder}, nil, []Point{{X: 4, Y: 1}}, 100)
	if err != nil {
		t.Fatalf("strength plan: %v", err)
	}
	if push.MovableID != 4 || push.Stand != (Point{X: 2, Y: 0}) ||
		push.From != boulder.Pos || push.To != (Point{X: 2, Y: 2}) ||
		push.Direction != (NativeStep{DY: 1}) {
		t.Fatalf("push %+v did not clear the corridor", push)
	}
	if len(push.Walk) == 0 {
		t.Fatal("plan omitted walking to the push stand")
	}
}

func TestNativeStrengthDoesNotPushIntoOccupiedTile(t *testing.T) {
	grid := nativeStrengthTestGrid(t)
	boulder := Movable{ID: 4, Pos: Point{X: 2, Y: 1}}
	_, err := FindNativeStrengthPush(grid, Point{X: 0, Y: 1}, []Movable{boulder},
		map[[2]int]bool{{2, 2}: true}, []Point{{X: 4, Y: 1}}, 100)
	if !errors.Is(err, ErrNoPath) {
		t.Fatalf("sealed alcove err=%v, want no safe push", err)
	}
}

func TestNativeStrengthNoActionIfGoalAlreadyReachable(t *testing.T) {
	grid := nativeStrengthTestGrid(t)
	boulder := Movable{ID: 4, Pos: Point{X: 2, Y: 1}}
	_, err := FindNativeStrengthPush(grid, Point{X: 0, Y: 1},
		[]Movable{boulder}, nil, []Point{{X: 1, Y: 1}}, 100)
	if !errors.Is(err, ErrNoPath) {
		t.Fatalf("unneeded Strength err=%v, want no plan", err)
	}
}

func TestNativeStrengthNeverTreatsOtherSpriteAsMovable(t *testing.T) {
	grid := nativeStrengthTestGrid(t)
	_, err := FindNativeStrengthPush(grid, Point{X: 0, Y: 1}, nil,
		map[[2]int]bool{{2, 1}: true}, []Point{{X: 4, Y: 1}}, 100)
	if !errors.Is(err, ErrNoPath) {
		t.Fatalf("ordinary NPC blocking corridor returned %v", err)
	}
}
