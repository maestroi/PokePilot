package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/forcedmove"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

func planForcedPathBesideWarp(t *testing.T, romData []byte, mapID uint8, sx, sy, warpX, warpY int) []fieldPathStep {
	t.Helper()
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		t.Fatalf("ParseMap(%02x): %v", mapID, err)
	}
	g, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(%02x): %v", mapID, err)
	}
	rules := fieldPathRules{ForcedLanding: func(x, y int) (world.Point, bool) {
		landing, ok := forcedmove.Landing(mapID, x, y)
		if !ok {
			return world.Point{}, false
		}
		return world.Point{X: landing.X, Y: landing.Y}, true
	}}
	var lastErr error
	for _, side := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		dx, dy := warpX+side.DX, warpY+side.DY
		if !g.InBounds(dx, dy) || !g.Walkable(dx, dy) {
			continue
		}
		plan, err := planFieldPath(g, nil, h.Tileset, sx, sy, dx, dy, nil, false, false, false, rules)
		if err == nil {
			return plan
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = world.ErrNoPath
	}
	t.Fatalf("no forced-movement path from (%d,%d) beside warp (%d,%d) on %02x: %v", sx, sy, warpX, warpY, mapID, lastErr)
	return nil
}

func requireForcedAction(t *testing.T, plan []fieldPathStep) {
	t.Helper()
	for _, step := range plan {
		if step.Action == fieldPathForced {
			return
		}
	}
	t.Fatalf("route has no forced-movement edge: %+v", plan)
}

func TestSharedForcedPlannerRoutesActualRocketFloors(t *testing.T) {
	romData := rocketHideoutROM(t)
	tests := []struct {
		name           string
		mapID          uint8
		startX, startY int
		warpX, warpY   int
	}{
		{name: "B2F entry to B3F", mapID: rocketHideoutB2FMap, startX: 27, startY: 8, warpX: 21, warpY: 8},
		{name: "B3F entry to B4F", mapID: rocketHideoutB3FMap, startX: 25, startY: 6, warpX: 19, warpY: 18},
		{name: "B3F reverse to B2F", mapID: rocketHideoutB3FMap, startX: 19, startY: 17, warpX: 25, warpY: 6},
		{name: "B2F stair to elevator", mapID: rocketHideoutB2FMap, startX: 21, startY: 9, warpX: 24, warpY: 19},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := planForcedPathBesideWarp(t, romData, tc.mapID, tc.startX, tc.startY, tc.warpX, tc.warpY)
			requireForcedAction(t, plan)
		})
	}
}

func TestSharedForcedPlannerDoesNotWalkThroughArrowTrigger(t *testing.T) {
	land := newFakeFieldPathGrid(5, 1)
	openCells(land, [2]int{0, 0}, [2]int{1, 0}, [2]int{2, 0}, [2]int{3, 0}, [2]int{4, 0})
	rules := fieldPathRules{ForcedLanding: func(x, y int) (world.Point, bool) {
		if x == 1 && y == 0 {
			return world.Point{X: 0, Y: 0}, true
		}
		return world.Point{}, false
	}}
	_, err := planFieldPath(land, nil, overworldTileset, 0, 0, 4, 0, nil, false, false, false, rules)
	if !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("forced trigger treated as ordinary walkable tile: err=%v", err)
	}
}
