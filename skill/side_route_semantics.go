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
	route2GateMap        uint8 = 0x31
	route10Map           uint8 = 0x15
	powerPlantMap        uint8 = 0x53
	powerPlantWarpX      uint8 = 6
	powerPlantWarpY      uint8 = 39
	ceruleanCaveB1FMap   uint8 = 0xE3
	ceruleanCave1FMap    uint8 = 0xE4
	ceruleanCaveB1FWarpX uint8 = 0
	ceruleanCaveB1FWarpY uint8 = 6
)

// redSideRouteTransitionForEdge models optional-world entrances whose door is
// real but lives in a different immutable walking component from the ordinary
// route. These are PivotOnly transitions: Cut/Surf is needed to bridge into
// the pocket, but a resumed save already standing on that pocket must remain
// able to use the ordinary warp without owning the field capability.
func redSideRouteTransitionForEdge(edge world.Edge) (gameruntime.Transition, bool) {
	if edge.Kind != world.EdgeWarp {
		return gameruntime.Transition{}, false
	}
	switch {
	case edge.From == semanticRoute2Map && edge.To == route2GateMap &&
		((edge.WarpX == 16 && edge.WarpY == 35) || (edge.WarpX == 15 && edge.WarpY == 39)):
		t := semanticTransition("red:route2_gate_cut", edge, capCanCut)
		t.PivotOnly = true
		return t, true

	case edge.From == route10Map && edge.To == powerPlantMap &&
		edge.WarpX == powerPlantWarpX && edge.WarpY == powerPlantWarpY:
		t := semanticTransition("red:power_plant_surf", edge, capCanSurf)
		t.PivotOnly = true
		return t, true

	case edge.From == ceruleanCave1FMap && edge.To == ceruleanCaveB1FMap &&
		edge.WarpX == ceruleanCaveB1FWarpX && edge.WarpY == ceruleanCaveB1FWarpY:
		// The lowest-floor ladder is a real ROM warp, but the 1F route to it
		// crosses the cave's lake. Red/Blue's own traversal therefore needs
		// Surf inside Cerulean Cave; treating the ladder as ordinary immutable
		// walking leaves B1F isolated even though 1F/2F are reachable.
		t := semanticTransition("red:cerulean_cave_b1f_surf", edge, capCanSurf)
		t.PivotOnly = true
		return t, true
	}
	return gameruntime.Transition{}, false
}

func (x *redRouteTransitionExecutor) executeSideRouteTransition(edge world.Edge, transition gameruntime.Transition) (world.TransitionExecutionResult, bool, error) {
	switch transition.ID {
	case "red:route2_gate_cut":
		if blockage := x.liveTransitionBlockage(transition); blockage != nil {
			return world.TransitionExecutionResult{}, true, blockage
		}
		result, err := x.executeCutWarpApproach(edge)
		return result, true, err

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

// executeCutWarpApproach clears one reachable Cut tree only when the target
// building warp is not already reachable through current live geometry. The
// generic Cut recovery already validates the actual front tile against RAM,
// cuts it, and steps onto the cleared cell; Changed then forces GoTo to rebuild
// live topology before it attempts the door again.
func (x *redRouteTransitionExecutor) executeCutWarpApproach(edge world.Edge) (world.TransitionExecutionResult, error) {
	if edge.Kind != world.EdgeWarp {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Cut warp approach %02x->%02x is not a warp", edge.From, edge.To)
	}
	if got := x.m.Peek8(sym.CurMap); got != edge.From {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Cut warp approach starts on %02x, current map is %02x", edge.From, got)
	}
	h, err := rom.ParseMap(x.romData, edge.From)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Cut warp approach parse map %02x: %w", edge.From, err)
	}
	grid, err := liveMapGrid(x.m, x.romData, h)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Cut warp approach build map %02x: %w", edge.From, err)
	}
	sx, sy := playerXY(x.m)
	if _, _, _, _, err := warpTarget(h, edge, grid, int(sx), int(sy), spriteBlockers(x.m), nil, x.romData); err == nil {
		return world.TransitionExecutionResult{}, nil
	}
	opened, err := cutThroughReachableTree(x.m, x.romData)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Cut warp approach: %w", err)
	}
	if !opened {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Cut warp approach to %02x has no reachable verified Cut tree", edge.To)
	}
	return world.TransitionExecutionResult{Changed: true}, nil
}

// executeSurfWarpApproach enters Surf at the first water-only step on a path
// to a real warp. It deliberately does not traverse the warp itself. Once Surf
// is positively observed, Changed makes GoTo overlay the live water topology;
// the ordinary Traverse path can then reach and own the door normally.
func (x *redRouteTransitionExecutor) executeSurfWarpApproach(edge world.Edge) (world.TransitionExecutionResult, error) {
	if edge.Kind != world.EdgeWarp {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach %02x->%02x is not a warp", edge.From, edge.To)
	}
	if got := x.m.Peek8(sym.CurMap); got != edge.From {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach starts on %02x, current map is %02x", edge.From, got)
	}
	h, err := rom.ParseMap(x.romData, edge.From)
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
	if x.m.Peek8(sym.WalkBikeSurfState) == fieldSurfingState {
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
	if err := walkWithinMap(x.m, x.romData, Destination{Map: edge.From, X: uint8(standX), Y: uint8(standY)}); err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach reach shoreline (%d,%d): %w", standX, standY, err)
	}
	if err := Face(x.m, uint8(waterX), uint8(waterY)); err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach face water (%d,%d): %w", waterX, waterY, err)
	}
	x.m.StepFrames(2)
	result, err := UseFieldMove(x.m, FieldSurf)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach enter mode: %w", err)
	}
	if !result.Surfing || x.m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf warp approach returned without verified surfing state")
	}
	return world.TransitionExecutionResult{Changed: true}, nil
}
