package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// rocketElevatorFloorEdge reports the three Rocket Hideout floor -> elevator
// edges whose warp activation is governed by ExtraWarpCheck's function 2.
// Merely reaching one of these warp coordinates is not enough: after stepping
// onto it, the tile in front of Red must be one of the facing direction's
// warp-carpet tiles (pokered/home/overworld.asm, IsWarpTileInFrontOfPlayer).
func rocketElevatorFloorEdge(edge world.Edge) bool {
	if edge.Kind != world.EdgeWarp || edge.To != rocketHideoutElevatorMap {
		return false
	}
	switch edge.From {
	case rocketHideoutB1FMap, rocketHideoutB2FMap, rocketHideoutB4FMap:
		return true
	default:
		return false
	}
}

// rocketWarpCarpetAllows is the literal WarpTileListPointers table from
// pokered/data/tilesets/warp_carpet_tile_ids.asm, expressed in game movement
// coordinates. push is the step used to ENTER the warp; once Red lands on the
// warp square he is still facing push, so the next cell in that direction is
// exactly what GetTileAndCoordsInFrontOfPlayer exposes as wTileInFrontOfPlayer.
func rocketWarpCarpetAllows(push world.Step, tile uint8) bool {
	switch push {
	case (world.Step{DX: 0, DY: 1}): // facing down
		switch tile {
		case 0x01, 0x12, 0x17, 0x3d, 0x04, 0x18, 0x33:
			return true
		}
	case (world.Step{DX: 0, DY: -1}): // facing up
		return tile == 0x01 || tile == 0x5c
	case (world.Step{DX: -1, DY: 0}): // facing left
		return tile == 0x1a || tile == 0x4b
	case (world.Step{DX: 1, DY: 0}): // facing right
		return tile == 0x0f || tile == 0x4e
	}
	return false
}

type rocketElevatorApproach struct {
	warpX, warpY int
	standX       int
	standY       int
	push         world.Step
	steps        []world.Step
}

// planRocketElevatorApproach finds an approach that satisfies the ROM's extra
// warp predicate, not merely geometry. This is the missing fact in generic
// warpTarget: on B1F/B2F/B4F a sideways step can put Red on the elevator warp
// coordinate while ExtraWarpCheck rejects it, leaving him parked on the warp
// forever. Grid.FieldTile is deliberately the same top-left subtile contract
// used by wTileInFrontOfPlayer, so this planner can reproduce the check exactly
// without guessing from pixels or collision ids.
func planRocketElevatorApproach(h rom.MapHeader, grid *world.Grid, sx, sy int, blocked map[[2]int]bool) (rocketElevatorApproach, error) {
	approachBlocked := make(map[[2]int]bool, len(blocked)+len(h.Warps))
	for p, value := range blocked {
		approachBlocked[p] = value
	}
	for _, warp := range h.Warps {
		if int(warp.X) == sx && int(warp.Y) == sy {
			continue
		}
		approachBlocked[[2]int{int(warp.X), int(warp.Y)}] = true
	}

	pushes := []world.Step{
		{DX: 0, DY: -1},
		{DX: 0, DY: 1},
		{DX: -1, DY: 0},
		{DX: 1, DY: 0},
	}
	bestLen := -1
	var best rocketElevatorApproach
	for _, warp := range h.Warps {
		if warp.DestMap != rocketHideoutElevatorMap {
			continue
		}
		wx, wy := int(warp.X), int(warp.Y)
		if !grid.Walkable(wx, wy) || blocked[[2]int{wx, wy}] {
			continue
		}
		for _, push := range pushes {
			frontX, frontY := wx+push.DX, wy+push.DY
			frontTile, ok := grid.FieldTile(frontX, frontY)
			if !ok || !rocketWarpCarpetAllows(push, frontTile) {
				continue
			}
			standX, standY := wx-push.DX, wy-push.DY
			if !grid.Walkable(standX, standY) || blocked[[2]int{standX, standY}] {
				continue
			}
			if !grid.Passable(standX, standY, wx, wy) {
				continue
			}
			steps, err := world.FindPath(grid, sx, sy, standX, standY, approachBlocked)
			if err != nil {
				continue
			}
			if bestLen >= 0 && len(steps) >= bestLen {
				continue
			}
			bestLen = len(steps)
			best = rocketElevatorApproach{
				warpX: wx, warpY: wy,
				standX: standX, standY: standY,
				push: push,
				steps: steps,
			}
		}
	}
	if bestLen < 0 {
		return rocketElevatorApproach{}, fmt.Errorf("no reachable Rocket elevator warp has a valid warp-carpet approach from (%d,%d)", sx, sy)
	}
	return best, nil
}

// traverseRocketElevatorFloorWarp performs a floor -> Rocket elevator edge.
// It first places Red on the side from which stepping onto the warp satisfies
// ExtraWarpCheck, then delegates the actual crossing/arrival verification to
// Traverse. Starting Traverse from that exact adjacent cell makes warpTarget's
// zero-length approach deterministic and keeps all existing map-flip checks in
// one place.
func traverseRocketElevatorFloorWarp(m *emu.Emu, romData []byte, edge world.Edge) error {
	if !rocketElevatorFloorEdge(edge) {
		return fmt.Errorf("skill: Rocket elevator carpet traversal does not own edge %02x->%02x", edge.From, edge.To)
	}
	if got := m.Peek8(sym.CurMap); got == edge.To {
		return nil
	} else if got != edge.From {
		return fmt.Errorf("skill: Rocket elevator carpet traversal on map %02x, want %02x", got, edge.From)
	}

	h, err := rom.ParseMap(romData, edge.From)
	if err != nil {
		return fmt.Errorf("skill: Rocket elevator parse floor %02x: %w", edge.From, err)
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return fmt.Errorf("skill: Rocket elevator build live floor %02x: %w", edge.From, err)
	}

	var approach rocketElevatorApproach
	err = walkAround(func() error { return movementInterruption(m) }, func() map[[2]int]bool { return spriteBlockers(m) },
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			x, y := playerXY(m)
			planned, planErr := planRocketElevatorApproach(h, grid, int(x), int(y), blocked)
			if planErr != nil {
				return nil, planErr
			}
			approach = planned
			return planned.steps, nil
		}, func(steps []world.Step) error { return WalkPath(m, steps) },
		func() { m.StepFrames(npcWaitFrames) })
	if err != nil {
		return fmt.Errorf("skill: Rocket elevator reach valid carpet approach: %w", err)
	}

	x, y := playerXY(m)
	if int(x) != approach.standX || int(y) != approach.standY {
		return fmt.Errorf("skill: Rocket elevator approach ended at (%d,%d), want (%d,%d)", x, y, approach.standX, approach.standY)
	}
	frontTile, ok := grid.FieldTile(approach.warpX+approach.push.DX, approach.warpY+approach.push.DY)
	if !ok || !rocketWarpCarpetAllows(approach.push, frontTile) {
		return fmt.Errorf("skill: Rocket elevator approach lost warp-carpet postcondition at (%d,%d)", approach.warpX, approach.warpY)
	}

	chosen := edge
	chosen.WarpX = uint8(approach.warpX)
	chosen.WarpY = uint8(approach.warpY)
	if err := Traverse(m, romData, chosen); err != nil {
		return fmt.Errorf("skill: Rocket elevator carpet crossing: %w", err)
	}
	return nil
}
