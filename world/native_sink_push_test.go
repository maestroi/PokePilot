package world

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func TestNativeStrengthSinkRequiresExactObjectAndRetailPit(t *testing.T) {
	spec := worldmodel.NativeGridSpec{
		MapID: 0x1801, Width: 5, Height: 3,
		Walkable: []bool{
			true, true, true, true, true,
			true, true, true, true, true,
			true, true, true, true, true,
		},
		CollisionTile: []uint8{
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0,
			0, 0, 0x60, 0, 0,
		},
	}
	g, err := NativeGridFromSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	sink := Point{X: 2, Y: 2}
	movables := []Movable{{ID: 4, Pos: Point{X: 2, Y: 1}}}
	// A sink also appears as a static warp blocker. The solver must
	// nevertheless allow the matching boulder to fall into the hole.
	fixed := map[[2]int]bool{{2, 2}: true, {2, 1}: true}
	push, err := FindNativeStrengthSinkPush(g, Point{X: 1, Y: 1}, movables,
		fixed, map[int]Point{4: sink}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if push.MovableID != 4 || push.From != (Point{X: 2, Y: 1}) ||
		push.To != sink || push.Stand != (Point{X: 2, Y: 0}) {
		t.Fatalf("unexpected pit push: %+v", push)
	}
	if _, err := FindNativeStrengthSinkPush(g, Point{X: 1, Y: 1}, movables,
		fixed, map[int]Point{5: sink}, 1000); !errors.Is(err, ErrNoPath) {
		t.Fatalf("wrong object must not use another boulder's hole: %v", err)
	}
	if _, err := FindNativeStrengthSinkPush(g, Point{X: 1, Y: 1}, movables,
		fixed, map[int]Point{4: {X: 4, Y: 2}}, 1000); err == nil {
		t.Fatal("non-pit destination was accepted as a warp sink")
	}
}

func TestNativeStrengthSinkCanClearOtherBoulderBeforeCorrectOne(t *testing.T) {
	spec := worldmodel.NativeGridSpec{
		MapID: 0x1802, Width: 5, Height: 3,
		Walkable: []bool{
			true, true, true, true, true,
			true, true, true, true, true,
			true, true, true, true, true,
		},
		CollisionTile: []uint8{
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0,
			0, 0, 0x60, 0, 0,
		},
	}
	g, err := NativeGridFromSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	movable := []Movable{
		{ID: 4, Pos: Point{X: 2, Y: 1}},
		{ID: 5, Pos: Point{X: 2, Y: 0}},
	}
	push, err := FindNativeStrengthSinkPush(g, Point{X: 1, Y: 0}, movable,
		map[[2]int]bool{{2, 2}: true}, map[int]Point{4: {X: 2, Y: 2}}, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if push.MovableID == 4 && push.To == (Point{X: 2, Y: 2}) {
		t.Fatal("boulder skipped the intervening blocking boulder")
	}
}
