package skill

import (
	"fmt"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	route2GateMap          = 0x31
	diglettsCaveRoute2Map  = 0x2E
	route2DiglettWarpX    uint8 = 12
	route2DiglettWarpY    uint8 = 9
	route10Map             = 0x15
	powerPlantMap          = 0x53
	powerPlantWarpX       uint8 = 6
	powerPlantWarpY       uint8 = 39
	ceruleanCaveB1FMap     = 0xE3
	ceruleanCave1FMap      = 0xE4
	ceruleanCaveB1FWarpX  uint8 = 0
	ceruleanCaveB1FWarpY  uint8 = 6
)

// redSideRouteTransitionForEdge models optional-world entrances whose door is
// real but lives in a different immutable walking component from the ordinary
// route. These are PivotOnly+PortBypass: Cut/Surf is needed to bridge into the
// pocket from the wrong component, but a resumed save already standing on that
// pocket must remain able to use the ordinary warp without owning the field
// capability (PivotOnly's missing-cap fallback).
func redSideRouteTransitionForEdge(edge world.Edge) (gameruntime.Transition, bool) {
	if edge.Kind != world.EdgeWarp || edge.From > 0xff || edge.To > 0xff {
		return gameruntime.Transition{}, false
	}
	from, to := uint8(edge.From), uint8(edge.To)
	switch {
	case from == semanticRoute2Map && to == route2GateMap &&
		((edge.WarpX == 16 && edge.WarpY == 35) || (edge.WarpX == 15 && edge.WarpY == 39)):
		t := semanticTransition("red:route2_gate_cut", edge, capCanCut)
		t.PivotOnly = true
		t.PortBypass = true
		return t, true

	case from == semanticRoute2Map && to == diglettsCaveRoute2Map &&
		edge.WarpX == route2DiglettWarpX && edge.WarpY == route2DiglettWarpY:
		// Diglett's Cave's Route 2 house opens onto a land pocket that only a
		// Cut tree joins to the rest of Route 2, the mainland's only non-Fly
		// link between Vermilion and Viridian. Same shape as the gate above.
		t := semanticTransition("red:route2_diglett_cut", edge, capCanCut)
		t.PivotOnly = true
		t.PortBypass = true
		return t, true

	case from == diglettsCaveRoute2Map && to == semanticRoute2Map:
		// Leaving the house lands in that pocket. PivotOnly keeps the door
		// ordinary and makes the landing a live-topology boundary, so GoTo
		// replans on Route 2 where the field planner can Cut out. Without it a
		// Fuchsia-side journey to Viridian or Pallet has no route at all
		// (run-2xj7ziq8p2p2o3siqjhbtm20e1, Secret Key via Pallet).
		t := semanticTransition("red:route2_diglett_cut", edge, capCanCut)
		t.PivotOnly = true
		return t, true

	case from == route10Map && to == powerPlantMap &&
		edge.WarpX == powerPlantWarpX && edge.WarpY == powerPlantWarpY:
		t := semanticTransition("red:power_plant_surf", edge, capCanSurf)
		t.PivotOnly = true
		t.PortBypass = true
		return t, true

	case from == ceruleanCave1FMap && to == ceruleanCaveB1FMap &&
		edge.WarpX == ceruleanCaveB1FWarpX && edge.WarpY == ceruleanCaveB1FWarpY:
		// The lowest-floor ladder is a real ROM warp, but the 1F route to it
		// crosses the cave's lake. Red/Blue's own traversal therefore needs
		// Surf inside Cerulean Cave; treating the ladder as ordinary immutable
		// walking leaves B1F isolated even though 1F/2F are reachable.
		t := semanticTransition("red:cerulean_cave_b1f_surf", edge, capCanSurf)
		t.PivotOnly = true
		t.PortBypass = true
		return t, true
	}
	return gameruntime.Transition{}, false
}

func (x *redRouteTransitionExecutor) executeSideRouteTransition(edge world.Edge, transition gameruntime.Transition) (world.TransitionExecutionResult, bool, error) {
	switch transition.ID {
	case "red:route2_gate_cut", "red:route2_diglett_cut":
		if blockage := x.liveTransitionBlockage(transition); blockage != nil {
			return world.TransitionExecutionResult{}, true, blockage
		}
		// Traverse will approach the selected gate warp with the shared field
		// planner and therefore Cut only a tree that actually unlocks this door.
		return world.TransitionExecutionResult{}, true, nil

	case "red:power_plant_surf", "red:cerulean_cave_b1f_surf":
		if blockage := x.liveTransitionBlockage(transition); blockage != nil {
			return world.TransitionExecutionResult{}, true, blockage
		}
		result, err := x.executeSurfWarpApproach(edge)
		return result, true, err
	}
	return world.TransitionExecutionResult{}, false, nil
}

func (x *redRouteTransitionExecutor) liveTransitionBlockage(transition gameruntime.Transition) error {
	var mem state.Mem
	state.Snapshot(x.m, &mem)
	blockage, usable := gameruntime.EvaluateTransition(transition, redRouteCapabilities(x.romData, &mem))
	if usable {
		return nil
	}
	return &blockage
}

// executeSurfWarpApproach enters Surf at the first water-only step on a path
// to a real warp. It deliberately does not traverse the warp itself. Once Surf
// is positively observed, Changed makes GoTo overlay the live water topology;
// the ordinary Traverse path can then reach and own the door normally.
func (x *redRouteTransitionExecutor) executeSurfWarpApproach(edge world.Edge) (world.TransitionExecutionResult, error) {
	if edge.Kind != world.EdgeWarp {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach %02x->%02x is not a warp", edge.From, edge.To)
	}
	if edge.From > 0xff || edge.To > 0xff {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach cannot execute wide map edge %04x->%04x", edge.From, edge.To)
	}
	from := uint8(edge.From)
	if got := x.m.Peek8(sym.CurMap); got != from {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach starts on %02x, current map is %02x", edge.From, got)
	}
	h, err := rom.ParseMap(x.romData, from)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach parse map %02x: %w", edge.From, err)
	}
	land, err := liveMapGridForTraversal(x.m, x.romData, h, world.TraversalLand)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach land grid: %w", err)
	}
	water, err := liveMapGridForTraversal(x.m, x.romData, h, world.TraversalWater)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach water grid: %w", err)
	}
	sx, sy := playerXY(x.m)
	blocked := spriteBlockers(x.m)
	if _, _, _, _, err := warpTarget(h, edge, land, int(sx), int(sy), blocked, nil, x.romData); err == nil {
		return world.TransitionExecutionResult{}, nil
	}
	_, _, steps, _, err := warpTarget(h, edge, water, int(sx), int(sy), blocked, nil, x.romData)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach cannot reach %02x even in water mode: %w", edge.To, err)
	}
	fieldActions, err := x.fieldActionDecoder()
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach field-action profile: %w", err)
	}
	if fieldActions.DecodeFieldAction(x.m).Surfing {
		return world.TransitionExecutionResult{}, nil
	}

	px, py := int(sx), int(sy)
	standX, standY, waterX, waterY := 0, 0, 0, 0
	found := false
	for _, step := range steps {
		nx, ny := px+step.DX, py+step.DY
		if !land.Passable(px, py, nx, ny) && water.Passable(px, py, nx, ny) {
			standX, standY, waterX, waterY = px, py, nx, ny
			found = true
			break
		}
		px, py = nx, ny
	}
	if !found {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach to %02x has no water-entry step", edge.To)
	}
	if err := walkWithinMap(x.m, x.romData, Destination{Map: from, X: uint8(standX), Y: uint8(standY)}); err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach reach shoreline (%d,%d): %w", standX, standY, err)
	}
	if err := Face(x.m, uint8(waterX), uint8(waterY)); err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach face water (%d,%d): %w", waterX, waterY, err)
	}
	x.m.StepFrames(2)
	result, err := useFieldMoveWithDecoder(x.m, FieldSurf, fieldActions)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach enter mode: %w", err)
	}
	if !result.Surfing || !fieldActions.DecodeFieldAction(x.m).Surfing {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach returned without verified surfing state")
	}
	return world.TransitionExecutionResult{Changed: true}, nil
}
