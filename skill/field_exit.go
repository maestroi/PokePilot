package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// openFieldPathTowardDestination opens one field-move-gated exit on the
// current map that progresses toward dest. It exists for the sealed-pocket
// shape measured on Celadon and Vermilion gym yards: the player already
// stands on the destination's city map, but Cut trees put them in a walking
// component that shares no land exit with the street. Component routing then
// reports world.ErrNoRoute because leave-and-return through the gym cannot
// relaxLanding onto an already-occupied map.
//
// The recovery is destination-aware and reuses Traverse's field planner: only
// exits that map-level routing can use toward dest.Map are considered, and
// only plans that require at least one Cut/Surf action count as "opening"
// (a zero-action walk would not change topology and would loop).
func openFieldPathTowardDestination(m *emu.Emu, romData []byte, g *world.Graph, dest Destination) (bool, error) {
	if m == nil || g == nil {
		return false, nil
	}
	cur := m.Peek8(sym.CurMap)
	if cur == dest.Map {
		return false, nil
	}

	type rankedExit struct {
		edge    world.Edge
		actions int
		moves   int
	}
	var best *rankedExit
	for _, e := range g.Edges[cur] {
		if e.From != cur {
			continue
		}
		if _, err := world.FindRoute(g, e.To, dest.Map); err != nil {
			continue
		}
		actions, moves, ok := fieldApproachCost(m, romData, e)
		if !ok || actions < 1 {
			continue
		}
		cand := rankedExit{edge: e, actions: actions, moves: moves}
		if best == nil || cand.actions < best.actions || (cand.actions == best.actions && cand.moves < best.moves) {
			best = &cand
		}
	}
	if best == nil {
		return false, nil
	}

	var err error
	switch best.edge.Kind {
	case world.EdgeWarp:
		err = approachWarpWithFieldPath(m, romData, best.edge)
	case world.EdgeConnection:
		err = approachConnectionWithFieldPath(m, romData, best.edge)
	default:
		return false, fmt.Errorf("skill: field-path exit on unsupported edge kind %d", best.edge.Kind)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// fieldApproachCost reports the cheapest field-path approach onto an edge's
// exit without walking it. ok is false when no capability-aware approach
// exists from the live standing tile.
func fieldApproachCost(m *emu.Emu, romData []byte, e world.Edge) (actions, moves int, ok bool) {
	switch e.Kind {
	case world.EdgeWarp:
		return warpFieldApproachCost(m, romData, e)
	case world.EdgeConnection:
		return connectionFieldApproachCost(m, romData, e)
	default:
		return 0, 0, false
	}
}

func warpFieldApproachCost(m *emu.Emu, romData []byte, e world.Edge) (actions, moves int, ok bool) {
	if e.Kind != world.EdgeWarp || m.Peek8(sym.CurMap) != e.From {
		return 0, 0, false
	}
	h, err := rom.ParseMap(romData, e.From)
	if err != nil {
		return 0, 0, false
	}
	candidates := edgeWarpCandidates(h, e, romData)
	if len(candidates) == 0 {
		return 0, 0, false
	}
	sx, sy := playerXY(m)
	blocked := spriteBlockers(m)
	blocked = warpAvoidance(h, int(sx), int(sy), blocked)

	bestActions, bestMoves := -1, -1
	for _, w := range candidates {
		for _, step := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
			ax, ay := int(w.X)+step.DX, int(w.Y)+step.DY
			if ax < 0 || ay < 0 || ax > 255 || ay > 255 {
				continue
			}
			dest := Destination{Map: e.From, X: uint8(ax), Y: uint8(ay)}
			plan, perr := currentFieldPathPlan(m, romData, h, dest, blocked)
			if perr != nil {
				continue
			}
			nActions := 0
			for _, p := range plan {
				if p.Action != fieldPathWalk {
					nActions++
				}
			}
			if bestActions < 0 || nActions < bestActions || (nActions == bestActions && len(plan) < bestMoves) {
				bestActions, bestMoves = nActions, len(plan)
			}
		}
	}
	if bestActions < 0 {
		return 0, 0, false
	}
	return bestActions, bestMoves, true
}

func connectionFieldApproachCost(m *emu.Emu, romData []byte, e world.Edge) (actions, moves int, ok bool) {
	if e.Kind != world.EdgeConnection || m.Peek8(sym.CurMap) != e.From {
		return 0, 0, false
	}
	h, err := rom.ParseMap(romData, e.From)
	if err != nil {
		return 0, 0, false
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return 0, 0, false
	}
	sx, sy := playerXY(m)
	blocked := currentObservedStationaryObjectBlockers(m, h)
	blocked = warpAvoidance(h, int(sx), int(sy), blocked)

	bestActions, bestMoves := -1, -1
	for _, at := range connectionFieldTargets(grid, e) {
		if !grid.Walkable(at[0], at[1]) || blocked[at] {
			continue
		}
		dest := Destination{Map: e.From, X: uint8(at[0]), Y: uint8(at[1])}
		plan, perr := currentFieldPathPlan(m, romData, h, dest, blocked)
		if perr != nil {
			continue
		}
		nActions := 0
		for _, step := range plan {
			if step.Action != fieldPathWalk {
				nActions++
			}
		}
		if bestActions < 0 || nActions < bestActions || (nActions == bestActions && len(plan) < bestMoves) {
			bestActions, bestMoves = nActions, len(plan)
		}
	}
	if bestActions < 0 {
		return 0, 0, false
	}
	return bestActions, bestMoves, true
}
