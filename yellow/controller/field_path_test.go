package controller

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

func yellowTestGrid(t *testing.T, walkable []bool, collision, field []uint8) *world.Grid {
	t.Helper()
	grid, err := world.GridFromSpec(worldmodel.GridSpec{
		MapID:        1,
		Width:        len(walkable),
		Height:       1,
		Walkable:     walkable,
		CollisionTile: collision,
		FieldTile:    field,
	})
	if err != nil {
		t.Fatalf("GridFromSpec: %v", err)
	}
	return grid
}

func TestPlanYellowFieldPathRequiresCutForTree(t *testing.T) {
	land := yellowTestGrid(t,
		[]bool{true, false, true},
		[]uint8{0x01, 0x3d, 0x01},
		[]uint8{0x01, 0x3d, 0x01},
	)
	water := yellowTestGrid(t,
		[]bool{true, false, true},
		[]uint8{0x01, 0x3d, 0x01},
		[]uint8{0x01, 0x3d, 0x01},
	)

	if _, err := planYellowFieldPath(land, water, 0, 0, 0, 2, 0, nil, false, false, false); !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("without Cut err=%v, want ErrNoPath", err)
	}
	plan, err := planYellowFieldPath(land, water, 0, 0, 0, 2, 0, nil, true, false, false)
	if err != nil {
		t.Fatalf("with Cut: %v", err)
	}
	if len(plan) != 2 || plan[0].Action != yellowFieldCut || plan[1].Action != yellowFieldWalk {
		t.Fatalf("plan=%+v, want Cut then walk", plan)
	}
}

func TestPlanYellowFieldPathRequiresSurfForWater(t *testing.T) {
	land := yellowTestGrid(t,
		[]bool{true, false, true},
		[]uint8{0x01, 0x14, 0x01},
		[]uint8{0x01, 0x14, 0x01},
	)
	water := yellowTestGrid(t,
		[]bool{true, true, true},
		[]uint8{0x01, 0x14, 0x01},
		[]uint8{0x01, 0x14, 0x01},
	)

	if _, err := planYellowFieldPath(land, water, 0, 0, 0, 2, 0, nil, false, false, false); !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("without Surf err=%v, want ErrNoPath", err)
	}
	plan, err := planYellowFieldPath(land, water, 0, 0, 0, 2, 0, nil, false, true, false)
	if err != nil {
		t.Fatalf("with Surf: %v", err)
	}
	if len(plan) != 2 || plan[0].Action != yellowFieldSurf || plan[1].Action != yellowFieldWalk {
		t.Fatalf("plan=%+v, want Surf then walk", plan)
	}
}
