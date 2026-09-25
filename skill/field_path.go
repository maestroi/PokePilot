package skill

import (
	"container/heap"
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
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
	actions  int
	moves    int
	weighted int
}

type fieldPathQueueNode struct {
	state fieldPathState
	cost  fieldPathCost
	index int
}

type fieldPathQueue struct {
	items  []*fieldPathQueueNode
	policy fieldPathCostPolicy
}

func (q fieldPathQueue) Len() int { return len(q.items) }
func (q fieldPathQueue) Less(i, j int) bool {
	return q.policy.less(q.items[i].cost, q.items[j].cost)
}
func (q fieldPathQueue) Swap(i, j int) {
	q.items[i], q.items[j] = q.items[j], q.items[i]
	q.items[i].index = i
	q.items[j].index = j
}
func (q *fieldPathQueue) Push(v any) {
	n := v.(*fieldPathQueueNode)
	n.index = len(q.items)
	q.items = append(q.items, n)
}
func (q *fieldPathQueue) Pop() any {
	n := len(q.items)
	v := q.items[n-1]
	q.items[n-1] = nil
	q.items = q.items[:n-1]
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

// planFieldPath preserves the historical conservative local policy for pure
// callers/tests: minimize field actions first, then movement. Runtime travel
// selects the policy-scoped variant below so speedrun mode can price Cut/Surf
// against walking instead of treating every field action as infinitely costly.
func planFieldPath(
	land, water fieldPathGrid,
	tileset uint8,
	sx, sy, dx, dy int,
	blocked map[[2]int]bool,
	canCut, canSurf, startWater bool,
	rules ...fieldPathRules,
) ([]fieldPathStep, error) {
	plan, _, err := planFieldPathWithCost(
		land, water, tileset,
		sx, sy, dx, dy,
		blocked,
		canCut, canSurf, startWater,
		conservativeFieldPathCostPolicy(),
		rules...,
	)
	return plan, err
}

// planFieldPathWithCost plans one local route while treating owned field moves
// as traversal capabilities. Conservative policy is lexicographic; fastest
// policy uses shared coarse travel-cost units. Cut appears as the step INTO
// the tree cell and Surf as the first land->water step. Runtime execution
// performs one field action and replans from fresh live state, so this planner
// never assumes a mutation succeeded.
func planFieldPathWithCost(
	land, water fieldPathGrid,
	tileset uint8,
	sx, sy, dx, dy int,
	blocked map[[2]int]bool,
	canCut, canSurf, startWater bool,
	costPolicy fieldPathCostPolicy,
	rules ...fieldPathRules,
) ([]fieldPathStep, fieldPathCost, error) {
	var rule fieldPathRules
	if len(rules) > 0 {
		rule = rules[0]
	}
	if land == nil || !land.InBounds(sx, sy) || !land.InBounds(dx, dy) || blocked[[2]int{sx, sy}] || blocked[[2]int{dx, dy}] {
		return nil, fieldPathCost{}, world.ErrNoPath
	}
	if sx == dx && sy == dy {
		return []fieldPathStep{}, fieldPathCost{}, nil
	}

	start := fieldPathState{x: sx, y: sy, water: startWater}
	best := map[fieldPathState]fieldPathCost{start: {}}
	parent := map[fieldPathState]fieldPathParent{}
	closed := map[fieldPathState]bool{}
	open := &fieldPathQueue{policy: costPolicy}
	heap.Push(open, &fieldPathQueueNode{state: start})

	push := func(from fieldPathState, to fieldPathState, step fieldPathStep, addMoves int) {
		if closed[to] {
			return
		}
		next := costPolicy.add(best[from], step.Action, addMoves)
		if old, seen := best[to]; seen && !costPolicy.less(next, old) {
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
					return nil, fieldPathCost{}, world.ErrNoPath
				}
				rev = append(rev, p.step)
				at = p.prev
			}
			out := make([]fieldPathStep, len(rev))
			for i := range rev {
				out[len(rev)-1-i] = rev[i]
			}
			return out, best[cur], nil
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
					fieldPathStep{Move: move, Action: fieldPathWalk}, absInt(move.DX)+absInt(move.DY))
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
						fieldPathStep{Move: input, Action: fieldPathForced, Landing: landing}, cost)
					continue
				}
				push(cur, fieldPathState{x: nx, y: ny},
					fieldPathStep{Move: move, Action: fieldPathWalk}, absInt(move.DX)+absInt(move.DY))
				continue
			}

			nx, ny := cur.x+input.DX, cur.y+input.DY
			if !land.InBounds(nx, ny) || blocked[[2]int{nx, ny}] {
				continue
			}
			if canCut && fieldPathCutTile(land, tileset, nx, ny) {
				push(cur, fieldPathState{x: nx, y: ny},
					fieldPathStep{Move: input, Action: fieldPathCut}, 1)
				continue
			}
			if canSurf && rule.canSurfFrom(cur.x, cur.y) && water != nil && fieldPathWaterTile(water, nx, ny) {
				if move, ok := water.Movement(cur.x, cur.y, input, blocked); ok {
					wx, wy := cur.x+move.DX, cur.y+move.DY
					push(cur, fieldPathState{x: wx, y: wy, water: true},
						fieldPathStep{Move: move, Action: fieldPathSurf}, absInt(move.DX)+absInt(move.DY))
				}
			}
		}
	}
	return nil, fieldPathCost{}, world.ErrNoPath
}

func currentFieldPathPlanWithRulesAndCost(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool, rules fieldPathRules) ([]fieldPathStep, fieldPathCost, error) {
	overworld, err := overworldDecoderFor(m)
	if err != nil {
		return nil, fieldPathCost{}, err
	}
	fieldActions, err := fieldActionDecoderFor(m)
	if err != nil {
		return nil, fieldPathCost{}, err
	}
	return currentFieldPathPlanWithRulesAndCostWithDecoders(m, overworld, fieldActions, romData, h, dest, blocked, rules)
}

func currentFieldPathPlanWithRulesAndCostWithDecoder(m *emu.Emu, decoder game.OverworldDecoder, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool, rules fieldPathRules) ([]fieldPathStep, fieldPathCost, error) {
	fieldActions, err := fieldActionDecoderFor(m)
	if err != nil {
		return nil, fieldPathCost{}, err
	}
	return currentFieldPathPlanWithRulesAndCostWithDecoders(m, decoder, fieldActions, romData, h, dest, blocked, rules)
}

func currentFieldPathPlanWithRulesAndCostWithDecoders(m *emu.Emu, decoder game.OverworldDecoder, fieldActions game.FieldActionDecoder, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool, rules fieldPathRules) ([]fieldPathStep, fieldPathCost, error) {
	land, err := liveMapGridForTraversal(m, romData, h, world.TraversalLand)
	if err != nil {
		return nil, fieldPathCost{}, err
	}
	water, err := liveMapGridForTraversal(m, romData, h, world.TraversalWater)
	if err != nil {
		return nil, fieldPathCost{}, err
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	caps := redRouteCapabilities(romData, &mem)
	startWater := fieldActions.DecodeFieldAction(m).Surfing
	live, err := fieldPathRuntimeStateWithDecoder(m, decoder)
	if err != nil {
		return nil, fieldPathCost{}, err
	}
	return planFieldPathWithCost(
		land, water, h.Tileset,
		int(live.X), int(live.Y), int(dest.X), int(dest.Y),
		blocked,
		caps.Has(capCanCut), caps.Has(capCanSurf), startWater,
		fieldPathCostPolicyFor(m),
		rules,
	)
}

func currentFieldPathPlanWithRules(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool, rules fieldPathRules) ([]fieldPathStep, error) {
	plan, _, err := currentFieldPathPlanWithRulesAndCost(m, romData, h, dest, blocked, rules)
	return plan, err
}

func currentFieldPathPlanWithCost(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool) ([]fieldPathStep, fieldPathCost, error) {
	return currentFieldPathPlanWithRulesAndCost(m, romData, h, dest, blocked, currentFieldPathRules(m, h))
}

func currentFieldPathPlanWithCostWithDecoder(m *emu.Emu, decoder game.OverworldDecoder, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool) ([]fieldPathStep, fieldPathCost, error) {
	fieldActions, err := fieldActionDecoderFor(m)
	if err != nil {
		return nil, fieldPathCost{}, err
	}
	return currentFieldPathPlanWithCostWithDecoders(m, decoder, fieldActions, romData, h, dest, blocked)
}

func currentFieldPathPlanWithCostWithDecoders(m *emu.Emu, decoder game.OverworldDecoder, fieldActions game.FieldActionDecoder, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool) ([]fieldPathStep, fieldPathCost, error) {
	return currentFieldPathPlanWithRulesAndCostWithDecoders(m, decoder, fieldActions, romData, h, dest, blocked, currentFieldPathRules(m, h))
}

func currentFieldPathPlan(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool) ([]fieldPathStep, error) {
	plan, _, err := currentFieldPathPlanWithCost(m, romData, h, dest, blocked)
	return plan, err
}

// fieldPathReachableOnCurrentMap is a geometry probe used before component
// routing. It uses routingBlockers + warpAvoidance: still-present stationary
// objects (including off-screen trainers) and the live sprite snapshot, with
// unrelated warp pads kept out of the walk. Using only on-screen observed
// stationary blockers falsely reports reachability through off-screen
// trainers and teleporter pads (Silph Co 5F Card Key), which then traps
// walkWithinMap in a sprite-fallback oscillation. A positive result means
// GoTo should stay on this map and let walkWithinMap execute the field
// actions directly; a negative result leaves the existing leave/re-enter
// component routing behavior untouched.
func fieldPathReachableOnCurrentMap(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination) (bool, error) {
	decoder, err := overworldDecoderFor(m)
	if err != nil {
		return false, err
	}
	return fieldPathReachableOnCurrentMapWithDecoder(m, decoder, romData, h, dest)
}

func fieldPathReachableOnCurrentMapWithDecoder(m *emu.Emu, decoder game.OverworldDecoder, romData []byte, h rom.MapHeader, dest Destination) (bool, error) {
	fieldActions, err := fieldActionDecoderFor(m)
	if err != nil {
		return false, err
	}
	return fieldPathReachableOnCurrentMapWithDecoders(m, decoder, fieldActions, romData, h, dest)
}

func fieldPathReachableOnCurrentMapWithDecoders(m *emu.Emu, decoder game.OverworldDecoder, fieldActions game.FieldActionDecoder, romData []byte, h rom.MapHeader, dest Destination) (bool, error) {
	live, err := fieldPathRuntimeStateWithDecoder(m, decoder)
	if err != nil {
		return false, err
	}
	if live.Map != dest.Map {
		return false, nil
	}
	blocked := routingBlockers(m, h)
	blocked = warpAvoidance(h, int(live.X), int(live.Y), blocked)
	_, _, err = currentFieldPathPlanWithCostWithDecoders(m, decoder, fieldActions, romData, h, dest, blocked)
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

// currentBlockingUndefeatedTrainer identifies the single undefeated,
// ordinary-class stationary trainer whose home tile is the only reason a
// local field path failed with world.ErrNoPath against the exact blocked set
// that produced that failure. Several Red interiors (Silph Co 5F's Rocket2
// guarding the Card Key room; Rocket Hideout's grunts) route the only
// corridor through a single STAY trainer's tile: pokered does not expect a
// walk around them, it expects their sight line to force the battle that
// then drops them from the object table. currentObservedStationaryObjectBlockers
// correctly marks that tile solid (it is, until fought), so a plain
// reachability probe reports a dead end instead of the fight the room
// actually wants.
//
// Exactly one candidate must provably reopen dest when its tile alone is
// removed from blocked; two or more, like currentLocalStrengthPlan's refusal
// to move a boulder that does not provably open the exact destination, means
// this probe cannot tell which one the room actually needs, so it declines
// rather than fight the wrong trainer.
func currentBlockingUndefeatedTrainer(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool) (rom.Object, bool, error) {
	if len(blocked) == 0 {
		return rom.Object{}, false, nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)

	var candidates []rom.Object
	for _, o := range h.Objects {
		if o.Movement != rom.MovementStay {
			continue
		}
		if !blocked[[2]int{int(o.X), int(o.Y)}] {
			continue
		}
		target, err := trainerTargetAt(romData, h, o.X, o.Y)
		if err != nil {
			// Not a standard-trainer object (item ball, clipboard, sign, an
			// NPC with a bespoke script): this probe only ever fights the
			// ordinary TalkToTrainer contract ChallengeTrainer supports.
			continue
		}
		if !ordinaryTrainerClass(target.object.TrainerClass) || target.flag.setMem(&mem) {
			continue
		}
		candidates = append(candidates, o)
	}
	return blockingUndefeatedTrainer(candidates, func(at [2]int) (bool, error) {
		relaxed := make(map[[2]int]bool, len(blocked))
		for k, v := range blocked {
			relaxed[k] = v
		}
		delete(relaxed, at)
		if _, err := currentFieldPathPlan(m, romData, h, dest, relaxed); err != nil {
			if errors.Is(err, world.ErrNoPath) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

// blockingUndefeatedTrainer is currentBlockingUndefeatedTrainer's pure
// selection rule, factored out so the "exactly one match wins, ties decline"
// contract is unit-testable without an emulator or ROM: candidates are
// already known to be undefeated, ordinary-class, and standing on a blocked
// tile, and reachableWithout reports whether dest becomes reachable with
// exactly that one tile unblocked.
func blockingUndefeatedTrainer(candidates []rom.Object, reachableWithout func(at [2]int) (bool, error)) (rom.Object, bool, error) {
	var candidate rom.Object
	matches := 0
	for _, o := range candidates {
		ok, err := reachableWithout([2]int{int(o.X), int(o.Y)})
		if err != nil {
			return rom.Object{}, false, err
		}
		if !ok {
			continue
		}
		matches++
		candidate = o
	}
	if matches != 1 {
		return rom.Object{}, false, nil
	}
	return candidate, true, nil
}

// fieldPathPlanActionCount reports how many non-walk operations a local field
// plan actually performs. Cross-map bridge recovery must require at least one:
// a zero-action plan is ordinary walking and belongs to the component/semantic
// router, not the field-bridge override.
func fieldPathPlanActionCount(plan []fieldPathStep) int {
	n := 0
	for _, step := range plan {
		if step.Action != fieldPathWalk {
			n++
		}
	}
	return n
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
	decoder, err := overworldDecoderFor(m)
	if err != nil {
		return err
	}
	return executeFieldPathActionWithDecoder(m, decoder, step)
}

func executeFieldPathActionWithDecoder(m *emu.Emu, decoder game.OverworldDecoder, step fieldPathStep) error {
	fieldActions, err := fieldActionDecoderFor(m)
	if err != nil {
		return err
	}
	return executeFieldPathActionWithDecoders(m, decoder, fieldActions, step)
}

func executeFieldPathActionWithDecoders(m *emu.Emu, decoder game.OverworldDecoder, fieldActions game.FieldActionDecoder, step fieldPathStep) error {
	live, tx, ty, err := fieldPathActionTargetWithDecoder(m, decoder, step)
	if err != nil {
		return err
	}
	if err := faceWithOverworldDecoder(m, decoder, uint8(tx), uint8(ty)); err != nil {
		return fmt.Errorf("skill: field path face (%d,%d): %w", tx, ty, err)
	}

	switch step.Action {
	case fieldPathCut:
		// Face only turns the sprite; the game refreshes its front-target
		// observation when the overworld considers a step, so press toward
		// the (blocking) tree once before reading it.
		if btn, ok := buttonFor(step.Move); ok {
			m.Tap(btn, 3, 7)
			m.StepFrames(8)
		}
		if !fieldActions.DecodeFieldAction(m).CuttableAhead {
			return fmt.Errorf("skill: field path planned Cut at (%d,%d), but live front target is not cuttable", tx, ty)
		}
		if _, err := useFieldMoveWithDecoder(m, FieldCut, fieldActions); err != nil {
			return fmt.Errorf("skill: field path Cut at (%d,%d): %w", tx, ty, err)
		}
		if err := stepOnceWithRuntimeDecoder(m, step.Move, decoder); err != nil {
			return fmt.Errorf("skill: field path enter cleared Cut tile (%d,%d): %w", tx, ty, err)
		}
		return nil

	case fieldPathForced:
		if err := executeForcedMovementStep(m, decoder, live.Map, step.Move, step.Landing); err != nil {
			return fmt.Errorf("skill: field path forced movement via (%d,%d) to (%d,%d): %w", tx, ty, step.Landing.X, step.Landing.Y, err)
		}
		return nil

	case fieldPathSurf:
		m.StepFrames(2)
		result, err := useFieldMoveWithDecoder(m, FieldSurf, fieldActions)
		if err != nil {
			return fmt.Errorf("skill: field path Surf toward (%d,%d): %w", tx, ty, err)
		}
		if !result.Surfing {
			return fmt.Errorf("skill: field path Surf toward (%d,%d) returned without verified surfing state", tx, ty)
		}
		return nil

	default:
		return fmt.Errorf("skill: field path execute called for non-action step")
	}
}
