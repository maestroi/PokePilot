package skill

import (
	"container/heap"
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

type fieldPathAction uint8

const (
	fieldPathWalk fieldPathAction = iota
	fieldPathCut
	fieldPathSurf
)

type fieldPathStep struct {
	Move   world.Step
	Action fieldPathAction
}

type fieldPathGrid interface {
	InBounds(x, y int) bool
	Walkable(x, y int) bool
	Movement(x, y int, input world.Step, blocked map[[2]int]bool) (world.Step, bool)
	Tile(x, y int) (uint8, bool)
	FieldTile(x, y int) (uint8, bool)
}

type fieldPathState struct {
	x, y  int
	water bool
}

type fieldPathCost struct {
	actions int
	moves   int
}

func (c fieldPathCost) less(other fieldPathCost) bool {
	if c.actions != other.actions {
		return c.actions < other.actions
	}
	return c.moves < other.moves
}

type fieldPathQueueNode struct {
	state fieldPathState
	cost  fieldPathCost
	index int
}

type fieldPathQueue []*fieldPathQueueNode

func (q fieldPathQueue) Len() int { return len(q) }
func (q fieldPathQueue) Less(i, j int) bool {
	if q[i].cost.actions != q[j].cost.actions {
		return q[i].cost.actions < q[j].cost.actions
	}
	return q[i].cost.moves < q[j].cost.moves
}
func (q fieldPathQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}
func (q *fieldPathQueue) Push(v any) {
	n := v.(*fieldPathQueueNode)
	n.index = len(*q)
	*q = append(*q, n)
}
func (q *fieldPathQueue) Pop() any {
	old := *q
	n := len(old)
	v := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return v
}

type fieldPathParent struct {
	prev fieldPathState
	step fieldPathStep
}

func fieldPathCutTile(g fieldPathGrid, tileset uint8, x, y int) bool {
	if field, ok := g.FieldTile(x, y); ok && cutRouteTile(tileset, field) {
		return true
	}
	if collision, ok := g.Tile(x, y); ok && cutRouteTile(tileset, collision) {
		return true
	}
	return false
}

func fieldPathWaterTile(g fieldPathGrid, x, y int) bool {
	if field, ok := g.FieldTile(x, y); ok && field == surfWaterTile {
		return true
	}
	if collision, ok := g.Tile(x, y); ok && collision == surfWaterTile {
		return true
	}
	return false
}

// planFieldPath plans one local route while treating owned field moves as
// traversal capabilities. Ordinary movement is preferred over field actions:
// the cost is lexicographic (fewest field actions, then fewest movement
// tiles), so Cut/Surf are used only when they unlock a route rather than as
// arbitrary shortcuts.
//
// Cut appears as the step INTO the tree cell. Surf appears as the first
// land->water step. Runtime execution performs one field action and replans
// from fresh live state, so this planner never assumes a mutation succeeded.
func planFieldPath(
	land, water fieldPathGrid,
	tileset uint8,
	sx, sy, dx, dy int,
	blocked map[[2]int]bool,
	canCut, canSurf, startWater bool,
) ([]fieldPathStep, error) {
	if land == nil || !land.InBounds(sx, sy) || !land.InBounds(dx, dy) || blocked[[2]int{sx, sy}] || blocked[[2]int{dx, dy}] {
		return nil, world.ErrNoPath
	}
	if sx == dx && sy == dy {
		return []fieldPathStep{}, nil
	}

	start := fieldPathState{x: sx, y: sy, water: startWater}
	best := map[fieldPathState]fieldPathCost{start: {}}
	parent := map[fieldPathState]fieldPathParent{}
	closed := map[fieldPathState]bool{}
	open := &fieldPathQueue{}
	heap.Push(open, &fieldPathQueueNode{state: start})

	push := func(from fieldPathState, to fieldPathState, step fieldPathStep, addActions, addMoves int) {
		if closed[to] {
			return
		}
		next := best[from]
		next.actions += addActions
		next.moves += addMoves
		if old, seen := best[to]; seen && !next.less(old) {
			return
		}
		best[to] = next
		parent[to] = fieldPathParent{prev: from, step: step}
		heap.Push(open, &fieldPathQueueNode{state: to, cost: next})
	}

	for open.Len() > 0 {
		curNode := heap.Pop(open).(*fieldPathQueueNode)
		cur := curNode.state
		if closed[cur] {
			continue
		}
		if known := best[cur]; curNode.cost != known {
			continue
		}
		closed[cur] = true
		if cur.x == dx && cur.y == dy {
			var rev []fieldPathStep
			for at := cur; at != start; {
				p, ok := parent[at]
				if !ok {
					return nil, world.ErrNoPath
				}
				rev = append(rev, p.step)
				at = p.prev
			}
			out := make([]fieldPathStep, len(rev))
			for i := range rev {
				out[len(rev)-1-i] = rev[i]
			}
			return out, nil
		}

		for _, input := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
			if cur.water {
				if water == nil {
					continue
				}
				move, ok := water.Movement(cur.x, cur.y, input, blocked)
				if !ok {
					continue
				}
				nx, ny := cur.x+move.DX, cur.y+move.DY
				nextWater := fieldPathWaterTile(water, nx, ny) || !land.Walkable(nx, ny)
				push(cur, fieldPathState{x: nx, y: ny, water: nextWater},
					fieldPathStep{Move: move, Action: fieldPathWalk}, 0, absInt(move.DX)+absInt(move.DY))
				continue
			}

			if move, ok := land.Movement(cur.x, cur.y, input, blocked); ok {
				nx, ny := cur.x+move.DX, cur.y+move.DY
				push(cur, fieldPathState{x: nx, y: ny},
					fieldPathStep{Move: move, Action: fieldPathWalk}, 0, absInt(move.DX)+absInt(move.DY))
				continue
			}

			nx, ny := cur.x+input.DX, cur.y+input.DY
			if !land.InBounds(nx, ny) || blocked[[2]int{nx, ny}] {
				continue
			}
			if canCut && fieldPathCutTile(land, tileset, nx, ny) {
				push(cur, fieldPathState{x: nx, y: ny},
					fieldPathStep{Move: input, Action: fieldPathCut}, 1, 1)
				continue
			}
			if canSurf && water != nil && fieldPathWaterTile(water, nx, ny) {
				if move, ok := water.Movement(cur.x, cur.y, input, blocked); ok {
					wx, wy := cur.x+move.DX, cur.y+move.DY
					push(cur, fieldPathState{x: wx, y: wy, water: true},
						fieldPathStep{Move: move, Action: fieldPathSurf}, 1, absInt(move.DX)+absInt(move.DY))
				}
			}
		}
	}
	return nil, world.ErrNoPath
}

func currentFieldPathPlan(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool) ([]fieldPathStep, error) {
	land, err := liveMapGridForTraversal(m, romData, h, world.TraversalLand)
	if err != nil {
		return nil, err
	}
	water, err := liveMapGridForTraversal(m, romData, h, world.TraversalWater)
	if err != nil {
		return nil, err
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	caps := redRouteCapabilities(romData, &mem)
	startWater := mem.U8(sym.WalkBikeSurfState) == fieldSurfingState
	sx, sy := playerXY(m)
	return planFieldPath(
		land, water, h.Tileset,
		int(sx), int(sy), int(dest.X), int(dest.Y),
		blocked,
		caps.Has(capCanCut), caps.Has(capCanSurf), startWater,
	)
}

// fieldPathReachableOnCurrentMap is a geometry probe used before component
// routing. It ignores transient moving sprites but keeps stationary observed
// blockers and unrelated warps out of the route. A positive result means GoTo
// should stay on this map and let walkWithinMap execute the field actions
// directly; a negative result leaves the existing leave/re-enter component
// routing behavior untouched.
func fieldPathReachableOnCurrentMap(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination) (bool, error) {
	if m.Peek8(sym.CurMap) != dest.Map {
		return false, nil
	}
	sx, sy := playerXY(m)
	blocked := currentObservedStationaryObjectBlockers(m, h)
	blocked = warpAvoidance(h, int(sx), int(sy), blocked)
	_, err := currentFieldPathPlan(m, romData, h, dest, blocked)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, world.ErrNoPath):
		return false, nil
	default:
		return false, err
	}
}

func firstFieldAction(plan []fieldPathStep) (prefix []world.Step, action *fieldPathStep) {
	for i := range plan {
		if plan[i].Action != fieldPathWalk {
			step := plan[i]
			return prefix, &step
		}
		prefix = append(prefix, plan[i].Move)
	}
	return prefix, nil
}

func executeFieldPathAction(m *emu.Emu, step fieldPathStep) error {
	px, py := playerXY(m)
	tx, ty := int(px)+step.Move.DX, int(py)+step.Move.DY
	if absInt(step.Move.DX)+absInt(step.Move.DY) != 1 {
		return fmt.Errorf("skill: field path action %d has non-adjacent step %s", step.Action, step.Move)
	}
	if err := Face(m, uint8(tx), uint8(ty)); err != nil {
		return fmt.Errorf("skill: field path face (%d,%d): %w", tx, ty, err)
	}

	switch step.Action {
	case fieldPathCut:
		if !cuttableFrontTile(observeFrontTile(m)) {
			return fmt.Errorf("skill: field path planned Cut at (%d,%d), but live front tile is not cuttable", tx, ty)
		}
		if _, err := UseFieldMove(m, FieldCut); err != nil {
			return fmt.Errorf("skill: field path Cut at (%d,%d): %w", tx, ty, err)
		}
		if err := StepOnce(m, step.Move); err != nil {
			return fmt.Errorf("skill: field path enter cleared Cut tile (%d,%d): %w", tx, ty, err)
		}
		return nil

	case fieldPathSurf:
		m.StepFrames(2)
		result, err := UseFieldMove(m, FieldSurf)
		if err != nil {
			return fmt.Errorf("skill: field path Surf toward (%d,%d): %w", tx, ty, err)
		}
		if !result.Surfing || m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
			return fmt.Errorf("skill: field path Surf toward (%d,%d) returned without verified surfing state", tx, ty)
		}
		return nil

	default:
		return fmt.Errorf("skill: field path execute called for non-action step")
	}
}
