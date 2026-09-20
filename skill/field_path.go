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
	fieldPathForced
)

type fieldPathStep struct {
	Move    world.Step
	Action  fieldPathAction
	Landing world.Point
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

type fieldPathRules struct {
	SurfAllowedFrom func(x, y int) bool
	ForcedLanding   func(x, y int) (world.Point, bool)
	MoveAllowed     func(x, y int, input world.Step) bool
}

func (r fieldPathRules) canSurfFrom(x, y int) bool {
	return r.SurfAllowedFrom == nil || r.SurfAllowedFrom(x, y)
}

func (r fieldPathRules) forcedLanding(x, y int) (world.Point, bool) {
	if r.ForcedLanding == nil {
		return world.Point{}, false
	}
	return r.ForcedLanding(x, y)
}

func (r fieldPathRules) moveAllowed(x, y int, input world.Step) bool {
	return r.MoveAllowed == nil || r.MoveAllowed(x, y, input)
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
	rules ...fieldPathRules,
) ([]fieldPathStep, error) {
	var rule fieldPathRules
	if len(rules) > 0 {
		rule = rules[0]
	}
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
			if !rule.moveAllowed(cur.x, cur.y, input) {
				continue
			}
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
				if landing, forced := rule.forcedLanding(nx, ny); forced {
					if !land.InBounds(landing.X, landing.Y) || !land.Walkable(landing.X, landing.Y) || blocked[[2]int{landing.X, landing.Y}] {
						continue
					}
					cost := absInt(move.DX) + absInt(move.DY) + absInt(landing.X-nx) + absInt(landing.Y-ny)
					push(cur, fieldPathState{x: landing.X, y: landing.Y},
						fieldPathStep{Move: input, Action: fieldPathForced, Landing: landing}, 0, cost)
					continue
				}
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
			if canSurf && rule.canSurfFrom(cur.x, cur.y) && water != nil && fieldPathWaterTile(water, nx, ny) {
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

func currentFieldPathPlanWithRules(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool, rules fieldPathRules) ([]fieldPathStep, error) {
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
		rules,
	)
}

func currentFieldPathPlan(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool) ([]fieldPathStep, error) {
	return currentFieldPathPlanWithRules(m, romData, h, dest, blocked, currentFieldPathRules(m, h))
}

// fieldPathReachableOnCurrentMap is a geometry probe used before component
// routing. It must agree with walkWithinMap's preferred blocker set
// (liveBlockers + warp avoidance): distant MovementStay homes are real solid
// geometry even when they are outside the sprite buffer. Probing only the
// nearby observed set can report reachable while the local walk then fails
// because a far NPC and an avoided warp tile jointly cut the floor — after
// which the sprite-only fallback plans through that NPC tile and still
// cannot arrive. Returning false here lets GoTo use leave/re-enter routing
// through ordinary warp edges instead.
func fieldPathReachableOnCurrentMap(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination) (bool, error) {
	if m.Peek8(sym.CurMap) != dest.Map {
		return false, nil
	}
	sx, sy := playerXY(m)
	blocked := liveBlockers(m, h)
	blocked = warpAvoidance(h, int(sx), int(sy), blocked)
	_, err := currentFieldPathPlan(m, romData, h, dest, blocked)
	switch {
	case err == nil:
		return true, nil
	case !errors.Is(err, world.ErrNoPath):
		return false, err
	}

	// Ordinary/Cut/Surf geometry is disconnected. Before allowing the map
	// router to leave and re-enter, ask whether live Strength boulders can open
	// a direct route to this same destination. Capability repair/execution is
	// deliberately deferred to walkWithinMap; this probe is geometry-only.
	_, needsStrength, strengthErr := currentLocalStrengthPlan(m, romData, h, dest)
	if strengthErr != nil {
		return false, strengthErr
	}
	return needsStrength, nil
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

	case fieldPathForced:
		if err := executeForcedMovementStep(m, m.Peek8(sym.CurMap), step.Move, step.Landing); err != nil {
			return fmt.Errorf("skill: field path forced movement via (%d,%d) to (%d,%d): %w", tx, ty, step.Landing.X, step.Landing.Y, err)
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
