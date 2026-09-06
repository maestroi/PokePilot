package world

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrPushPuzzleNoSolution = errors.New("world: push puzzle has no solution")
	ErrPushPuzzleStateLimit = errors.New("world: push puzzle state limit reached")
)

const defaultPushPuzzleMaxStates = 20000

// Point is a game-tile coordinate used by the movable-object planner.
type Point struct {
	X, Y int
}

// Movable is one pushable object. ID is stable within a loaded puzzle (for
// Pokémon Red this is the live sprite slot); Pos is its current tile.
type Movable struct {
	ID  int
	Pos Point
}

// PushGoal describes the state the planner must reach. Targets are tiles that
// must each contain some movable object. Reachable, when non-nil, must be
// reachable by the player after accounting for the final movable positions.
// When both are supplied, both conditions must hold.
type PushGoal struct {
	Targets   []Point
	Reachable *Point
}

// PushPuzzle is one observed movable-object problem. Grid is immutable/live
// map collision, Fixed contains non-movable object blockers, and Movables are
// the pushable objects. MaxStates bounds unattended search; zero uses a safe
// default.
type PushPuzzle struct {
	Grid      *Grid
	Player    Point
	Movables  []Movable
	Fixed     map[[2]int]bool
	Goal      PushGoal
	MaxStates int
}

// Push is one planned push plus the ordinary walking required to get behind
// the movable first. Direction moves the object From -> To and the player
// Stand -> From.
type Push struct {
	MovableID int
	Stand     Point
	From      Point
	To        Point
	Direction Step
	Walk      []Step
}

// PushPlan is a shortest-by-push-count deterministic solution. FinalWalk is
// populated only when Goal.Reachable is used; callers solving only a switch
// target can ignore it.
type PushPlan struct {
	Pushes    []Push
	FinalWalk []Step
	Explored  int
}

// PushPuzzleError carries bounded-search diagnostics without forcing callers
// to parse an error string. errors.Is works with ErrPushPuzzleNoSolution and
// ErrPushPuzzleStateLimit through Unwrap.
type PushPuzzleError struct {
	Kind      error
	Explored  int
	MaxStates int
	Player    Point
	Movables  []Movable
}

func (e *PushPuzzleError) Error() string {
	return fmt.Sprintf("%v after %d states (limit %d), player=(%d,%d), movables=%v",
		e.Kind, e.Explored, e.MaxStates, e.Player.X, e.Player.Y, e.Movables)
}

func (e *PushPuzzleError) Unwrap() error { return e.Kind }

type pushPuzzleState struct {
	Player   Point
	Movables []Movable // always sorted by ID
}

type pushPuzzleNode struct {
	State  pushPuzzleState
	Parent int
	Push   Push
}

func pointKey(p Point) [2]int { return [2]int{p.X, p.Y} }

func copyMovables(in []Movable) []Movable {
	out := make([]Movable, len(in))
	copy(out, in)
	return out
}

func normalizeMovables(in []Movable) ([]Movable, error) {
	out := copyMovables(in)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	for i := 1; i < len(out); i++ {
		if out[i-1].ID == out[i].ID {
			return nil, fmt.Errorf("world: duplicate movable id %d", out[i].ID)
		}
	}
	return out, nil
}

func pushStateKey(s pushPuzzleState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d,%d|", s.Player.X, s.Player.Y)
	for _, m := range s.Movables {
		fmt.Fprintf(&b, "%d:%d,%d;", m.ID, m.Pos.X, m.Pos.Y)
	}
	return b.String()
}

func stateBlockers(fixed map[[2]int]bool, movables []Movable) map[[2]int]bool {
	out := make(map[[2]int]bool, len(fixed)+len(movables))
	for p := range fixed {
		out[p] = true
	}
	for _, m := range movables {
		out[pointKey(m.Pos)] = true
	}
	return out
}

func movableOccupancy(movables []Movable) map[[2]int]bool {
	out := make(map[[2]int]bool, len(movables))
	for _, m := range movables {
		out[pointKey(m.Pos)] = true
	}
	return out
}

func goalTargetSet(goal PushGoal) map[[2]int]bool {
	out := make(map[[2]int]bool, len(goal.Targets))
	for _, target := range goal.Targets {
		out[pointKey(target)] = true
	}
	return out
}

func puzzleGoalSatisfied(p PushPuzzle, state pushPuzzleState) (bool, []Step) {
	occupied := movableOccupancy(state.Movables)
	for _, target := range p.Goal.Targets {
		if !occupied[pointKey(target)] {
			return false, nil
		}
	}
	if p.Goal.Reachable == nil {
		return true, nil
	}
	blocked := stateBlockers(p.Fixed, state.Movables)
	walk, err := FindPath(p.Grid, state.Player.X, state.Player.Y, p.Goal.Reachable.X, p.Goal.Reachable.Y, blocked)
	if err != nil {
		return false, nil
	}
	return true, walk
}

// IsStaticPushDeadlock reports the common irreversible corner deadlock: a
// movable is not on a target and has a static obstruction on at least one
// horizontal side and at least one vertical side. Other movables are excluded
// because they may themselves move later.
func IsStaticPushDeadlock(g *Grid, fixed map[[2]int]bool, pos Point, targets map[[2]int]bool) bool {
	if targets[pointKey(pos)] {
		return false
	}
	blocked := func(x, y int) bool {
		if !g.InBounds(x, y) || !g.Walkable(x, y) {
			return true
		}
		return fixed[[2]int{x, y}]
	}
	left := blocked(pos.X-1, pos.Y)
	right := blocked(pos.X+1, pos.Y)
	up := blocked(pos.X, pos.Y-1)
	down := blocked(pos.X, pos.Y+1)
	return (left || right) && (up || down)
}

func validatePushPuzzle(p PushPuzzle, movables []Movable) error {
	if p.Grid == nil {
		return fmt.Errorf("world: push puzzle has nil grid")
	}
	if !p.Grid.InBounds(p.Player.X, p.Player.Y) {
		return fmt.Errorf("world: push puzzle player (%d,%d) is out of bounds", p.Player.X, p.Player.Y)
	}
	if len(p.Goal.Targets) == 0 && p.Goal.Reachable == nil {
		return fmt.Errorf("world: push puzzle has no goal")
	}
	seenPos := map[[2]int]int{}
	for _, m := range movables {
		if !p.Grid.InBounds(m.Pos.X, m.Pos.Y) {
			return fmt.Errorf("world: movable %d at (%d,%d) is out of bounds", m.ID, m.Pos.X, m.Pos.Y)
		}
		key := pointKey(m.Pos)
		if other, exists := seenPos[key]; exists {
			return fmt.Errorf("world: movables %d and %d share (%d,%d)", other, m.ID, m.Pos.X, m.Pos.Y)
		}
		seenPos[key] = m.ID
		if p.Fixed[key] {
			return fmt.Errorf("world: movable %d overlaps fixed blocker at (%d,%d)", m.ID, m.Pos.X, m.Pos.Y)
		}
	}
	for _, target := range p.Goal.Targets {
		if !p.Grid.InBounds(target.X, target.Y) {
			return fmt.Errorf("world: push target (%d,%d) is out of bounds", target.X, target.Y)
		}
	}
	if r := p.Goal.Reachable; r != nil && !p.Grid.InBounds(r.X, r.Y) {
		return fmt.Errorf("world: reachable goal (%d,%d) is out of bounds", r.X, r.Y)
	}
	return nil
}

func reconstructPushPlan(nodes []pushPuzzleNode, index int, finalWalk []Step, explored int) PushPlan {
	var pushes []Push
	for index > 0 {
		node := nodes[index]
		pushes = append(pushes, node.Push)
		index = node.Parent
	}
	for i, j := 0, len(pushes)-1; i < j; i, j = i+1, j-1 {
		pushes[i], pushes[j] = pushes[j], pushes[i]
	}
	return PushPlan{Pushes: pushes, FinalWalk: finalWalk, Explored: explored}
}

// PlanPushPuzzle performs a bounded breadth-first search over push states.
// Ordinary walking is collapsed into reachability between pushes, which keeps
// the state space far smaller than searching every button press. The search is
// deterministic: movable IDs and direction order are stable, and BFS minimizes
// pushes before considering later alternatives.
//
// A target tile may be non-walkable. This is intentional for sink/hole goals:
// if the game allows a movable to enter that coordinate, planning can still
// express the terminal push. Non-target push destinations must be walkable.
func PlanPushPuzzle(p PushPuzzle) (PushPlan, error) {
	movables, err := normalizeMovables(p.Movables)
	if err != nil {
		return PushPlan{}, err
	}
	if err := validatePushPuzzle(p, movables); err != nil {
		return PushPlan{}, err
	}
	maxStates := p.MaxStates
	if maxStates <= 0 {
		maxStates = defaultPushPuzzleMaxStates
	}

	start := pushPuzzleState{Player: p.Player, Movables: movables}
	if ok, finalWalk := puzzleGoalSatisfied(p, start); ok {
		return PushPlan{FinalWalk: finalWalk}, nil
	}

	targets := goalTargetSet(p.Goal)
	nodes := []pushPuzzleNode{{State: start, Parent: -1}}
	queue := []int{0}
	visited := map[string]bool{pushStateKey(start): true}
	explored := 0

	for len(queue) > 0 {
		index := queue[0]
		queue = queue[1:]
		explored++
		if explored > maxStates {
			return PushPlan{}, &PushPuzzleError{
				Kind: ErrPushPuzzleStateLimit, Explored: explored - 1, MaxStates: maxStates,
				Player: start.Player, Movables: copyMovables(start.Movables),
			}
		}

		cur := nodes[index].State
		blocked := stateBlockers(p.Fixed, cur.Movables)
		for movableIndex, movable := range cur.Movables {
			for _, direction := range stepDirs {
				stand := Point{X: movable.Pos.X - direction.DX, Y: movable.Pos.Y - direction.DY}
				to := Point{X: movable.Pos.X + direction.DX, Y: movable.Pos.Y + direction.DY}
				if !p.Grid.InBounds(stand.X, stand.Y) || !p.Grid.Walkable(stand.X, stand.Y) || blocked[pointKey(stand)] {
					continue
				}
				if !p.Grid.InBounds(to.X, to.Y) || p.Fixed[pointKey(to)] || blocked[pointKey(to)] {
					continue
				}
				if !p.Grid.Walkable(to.X, to.Y) && !targets[pointKey(to)] {
					continue
				}

				walk, err := FindPath(p.Grid, cur.Player.X, cur.Player.Y, stand.X, stand.Y, blocked)
				if err != nil {
					continue
				}

				next := pushPuzzleState{Player: movable.Pos, Movables: copyMovables(cur.Movables)}
				next.Movables[movableIndex].Pos = to
				// Corner pruning is sound for pure target-occupancy puzzles: a
				// boulder parked in a non-target static corner can never help.
				// It is NOT sound for an exit-reachability goal, where parking a
				// boulder irreversibly in an alcove may be exactly what opens the
				// route. Disable this optimization whenever reachability is part
				// of the goal and let the bounded search decide.
				if p.Goal.Reachable == nil && IsStaticPushDeadlock(p.Grid, p.Fixed, to, targets) {
					continue
				}
				key := pushStateKey(next)
				if visited[key] {
					continue
				}
				visited[key] = true

				push := Push{
					MovableID: movable.ID,
					Stand:     stand,
					From:      movable.Pos,
					To:        to,
					Direction: direction,
					Walk:      walk,
				}
				nodes = append(nodes, pushPuzzleNode{State: next, Parent: index, Push: push})
				nextIndex := len(nodes) - 1
				if ok, finalWalk := puzzleGoalSatisfied(p, next); ok {
					return reconstructPushPlan(nodes, nextIndex, finalWalk, explored), nil
				}
				queue = append(queue, nextIndex)
			}
		}
	}

	return PushPlan{}, &PushPuzzleError{
		Kind: ErrPushPuzzleNoSolution, Explored: explored, MaxStates: maxStates,
		Player: start.Player, Movables: copyMovables(start.Movables),
	}
}
