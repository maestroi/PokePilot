package skill

import (
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

const (
	route2GateMap         uint8 = 0x31
	diglettsCaveRoute2Map uint8 = 0x2E
	route2DiglettWarpX    uint8 = 12
	route2DiglettWarpY    uint8 = 9
	route10Map            uint8 = 0x15
	powerPlantMap         uint8 = 0x53
	powerPlantWarpX       uint8 = 6
	powerPlantWarpY       uint8 = 39
	ceruleanCaveB1FMap    uint8 = 0xE3
	ceruleanCave1FMap     uint8 = 0xE4
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
	if edge.Kind != world.EdgeWarp {
		return gameruntime.Transition{}, false
	}
	switch {
	case edge.From == semanticRoute2Map && edge.To == route2GateMap &&
		((edge.WarpX == 16 && edge.WarpY == 35) || (edge.WarpX == 15 && edge.WarpY == 39)):
		t := semanticTransition("red:route2_gate_cut", edge, capCanCut)
		t.PivotOnly = true
		t.PortBypass = true
		return t, true

	case edge.From == semanticRoute2Map && edge.To == diglettsCaveRoute2Map &&
		edge.WarpX == route2DiglettWarpX && edge.WarpY == route2DiglettWarpY:
		// Diglett's Cave's Route 2 house opens onto a land pocket that only a
		// Cut tree joins to the rest of Route 2, the mainland's only non-Fly
		// link between Vermilion and Viridian. Same shape as the gate above.
		t := semanticTransition("red:route2_diglett_cut", edge, capCanCut)
		t.PivotOnly = true
		t.PortBypass = true
		return t, true

	case edge.From == diglettsCaveRoute2Map && edge.To == semanticRoute2Map:
		// Leaving the house lands in that pocket. PivotOnly keeps the door
		// ordinary and makes the landing a live-topology boundary, so GoTo
		// replans on Route 2 where the field planner can Cut out. Without it a
		// Fuchsia-side journey to Viridian or Pallet has no route at all
		// (run-2xj7ziq8p2p2o3siqjhbtm20e1, Secret Key via Pallet).
		t := semanticTransition("red:route2_diglett_cut", edge, capCanCut)
		t.PivotOnly = true
		return t, true

	case edge.From == route10Map && edge.To == powerPlantMap &&
		edge.WarpX == powerPlantWarpX && edge.WarpY == powerPlantWarpY:
		t := semanticTransition("red:power_plant_surf", edge, capCanSurf)
		t.PivotOnly = true
		t.PortBypass = true
		return t, true

	case edge.From == ceruleanCave1FMap && edge.To == ceruleanCaveB1FMap &&
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
	case "red:route2_gate_cut", "red:route2_diglett_cut",
		"red:power_plant_surf", "red:cerulean_cave_b1f_surf":
		if blockage := x.liveTransitionBlockage(transition); blockage != nil {
			return world.TransitionExecutionResult{}, true, blockage
		}
		// These are component pivots, not action executors. Traverse already
		// approaches warp edges through the shared destination-aware field
		// planner (approachWarpWithFieldPath), which owns both Cut and Surf,
		// refreshes the live grid after the field action, and then retries the
		// concrete warp. Keeping the semantic transition capability-only avoids
		// planning a whole approach in water mode before Surf is actually active.
		return world.TransitionExecutionResult{}, true, nil
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
