package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/world"
)

type fakeFieldPathGrid struct {
	w, h      int
	walkable  map[[2]int]bool
	fieldTile map[[2]int]uint8
	tile      map[[2]int]uint8
}

func newFakeFieldPathGrid(w, h int) *fakeFieldPathGrid {
	return &fakeFieldPathGrid{
		w:         w,
		h:         h,
		walkable:  map[[2]int]bool{},
		fieldTile: map[[2]int]uint8{},
		tile:      map[[2]int]uint8{},
	}
}

func (g *fakeFieldPathGrid) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.w && y < g.h
}

func (g *fakeFieldPathGrid) Walkable(x, y int) bool {
	return g.InBounds(x, y) && g.walkable[[2]int{x, y}]
}

func (g *fakeFieldPathGrid) Movement(x, y int, input world.Step, blocked map[[2]int]bool) (world.Step, bool) {
	nx, ny := x+input.DX, y+input.DY
	if blocked[[2]int{nx, ny}] || !g.Walkable(nx, ny) {
		return world.Step{}, false
	}
	return input, true
}

func (g *fakeFieldPathGrid) Tile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) {
		return 0, false
	}
	return g.tile[[2]int{x, y}], true
}

func (g *fakeFieldPathGrid) FieldTile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) {
		return 0, false
	}
	return g.fieldTile[[2]int{x, y}], true
}

func openCells(g *fakeFieldPathGrid, cells ...[2]int) {
	for _, c := range cells {
		g.walkable[c] = true
	}
}

func countFieldActions(plan []fieldPathStep, action fieldPathAction) int {
	n := 0
	for _, step := range plan {
		if step.Action == action {
			n++
		}
	}
	return n
}

func TestPlanFieldPathOpensGymCutChamberToDoorApproach(t *testing.T) {
	// Celadon Gym's leader pocket is a Cut-tree chamber: land-only FindPath
	// reports no route to the south door even when Cut is usable. Local field
	// pathing must treat the gym tree tile as a destination-aware Cut step.
	land := newFakeFieldPathGrid(5, 6)
	water := newFakeFieldPathGrid(5, 6)

	openCells(land,
		[2]int{2, 1}, [2]int{2, 2}, // sealed chamber
		[2]int{2, 4}, [2]int{2, 5}, // corridor to the door approach
	)
	for y := 0; y < 6; y++ {
		openCells(water, [2]int{2, y})
	}
	land.fieldTile[[2]int{2, 3}] = gymCutTreeTile

	plan, err := planFieldPath(land, water, gymTileset, 2, 1, 2, 5, nil, true, false, false)
	if err != nil {
		t.Fatalf("planFieldPath through gym Cut chamber: %v", err)
	}
	if got := countFieldActions(plan, fieldPathCut); got != 1 {
		t.Fatalf("Cut actions = %d, want 1; plan=%+v", got, plan)
	}
}

func TestPlanFieldPathCutsTreeOnSelectedRouteNotNearbyDeadEnd(t *testing.T) {
	land := newFakeFieldPathGrid(7, 3)
	water := newFakeFieldPathGrid(7, 3)

	for x := 0; x < 7; x++ {
		if x != 3 {
			openCells(land, [2]int{x, 1})
		}
		openCells(water, [2]int{x, 1})
	}
	// A tempting nearby tree leads only into a dead-end spur. The required
	// tree is the one that actually joins start to destination.
	openCells(land, [2]int{0, 0})
	land.fieldTile[[2]int{1, 0}] = cutTreeTile
	land.fieldTile[[2]int{3, 1}] = cutTreeTile

	plan, err := planFieldPath(land, water, overworldTileset, 0, 1, 6, 1, nil, true, false, false)
	if err != nil {
		t.Fatalf("planFieldPath: %v", err)
	}
	if got := countFieldActions(plan, fieldPathCut); got != 1 {
		t.Fatalf("Cut actions = %d, want exactly 1; plan=%+v", got, plan)
	}

	x, y := 0, 1
	cutAt := [2]int{-1, -1}
	for _, step := range plan {
		x += step.Move.DX
		y += step.Move.DY
		if step.Action == fieldPathCut {
			cutAt = [2]int{x, y}
		}
	}
	if cutAt != [2]int{3, 1} {
		t.Fatalf("planned Cut at %v, want route tree (3,1)", cutAt)
	}
}

func TestPlanFieldPathPrefersOrdinaryDetourOverUnneededCut(t *testing.T) {
	land := newFakeFieldPathGrid(5, 3)
	water := newFakeFieldPathGrid(5, 3)

	// Direct row contains a Cut tree, but the upper row is an ordinary
	// walkable detour. Field actions are intentionally secondary to walking.
	for x := 0; x < 5; x++ {
		openCells(land, [2]int{x, 0})
		openCells(water, [2]int{x, 0})
	}
	openCells(land, [2]int{0, 1}, [2]int{1, 1}, [2]int{3, 1}, [2]int{4, 1})
	openCells(water, [2]int{0, 1}, [2]int{1, 1}, [2]int{3, 1}, [2]int{4, 1})
	land.fieldTile[[2]int{2, 1}] = cutTreeTile

	plan, err := planFieldPath(land, water, overworldTileset, 0, 1, 4, 1, nil, true, false, false)
	if err != nil {
		t.Fatalf("planFieldPath: %v", err)
	}
	if got := countFieldActions(plan, fieldPathCut); got != 0 {
		t.Fatalf("Cut actions = %d, want 0 when ordinary route exists; plan=%+v", got, plan)
	}
}

func TestPlanFieldPathUsesSurfOnlyWhenWaterIsRequired(t *testing.T) {
	land := newFakeFieldPathGrid(5, 1)
	water := newFakeFieldPathGrid(5, 1)

	openCells(land, [2]int{0, 0}, [2]int{4, 0})
	for x := 0; x < 5; x++ {
		openCells(water, [2]int{x, 0})
	}
	for x := 1; x <= 3; x++ {
		water.fieldTile[[2]int{x, 0}] = surfWaterTile
	}

	plan, err := planFieldPath(land, water, overworldTileset, 0, 0, 4, 0, nil, false, true, false)
	if err != nil {
		t.Fatalf("planFieldPath with Surf: %v", err)
	}
	if got := countFieldActions(plan, fieldPathSurf); got != 1 {
		t.Fatalf("Surf actions = %d, want exactly 1; plan=%+v", got, plan)
	}
	if len(plan) == 0 || plan[0].Action != fieldPathSurf {
		t.Fatalf("first step = %+v, want Surf entry", plan)
	}

	_, err = planFieldPath(land, water, overworldTileset, 0, 0, 4, 0, nil, false, false, false)
	if !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("without Surf err=%v, want world.ErrNoPath", err)
	}
}

func TestPlanFieldPathHonorsSurfEntryRule(t *testing.T) {
	land := newFakeFieldPathGrid(5, 1)
	water := newFakeFieldPathGrid(5, 1)

	openCells(land, [2]int{0, 0}, [2]int{4, 0})
	for x := 0; x < 5; x++ {
		openCells(water, [2]int{x, 0})
	}
	for x := 1; x <= 3; x++ {
		water.fieldTile[[2]int{x, 0}] = surfWaterTile
	}

	denyStart := fieldPathRules{
		SurfAllowedFrom: func(x, y int) bool {
			return x != 0 || y != 0
		},
	}
	_, err := planFieldPath(land, water, overworldTileset, 0, 0, 4, 0, nil, false, true, false, denyStart)
	if !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("blocked Surf entry err=%v, want world.ErrNoPath", err)
	}

	plan, err := planFieldPath(land, water, overworldTileset, 0, 0, 4, 0, nil, false, true, false, fieldPathRules{})
	if err != nil {
		t.Fatalf("unrestricted Surf entry: %v", err)
	}
	if got := countFieldActions(plan, fieldPathSurf); got != 1 {
		t.Fatalf("Surf actions = %d, want 1; plan=%+v", got, plan)
	}
}
