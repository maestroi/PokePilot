package world

import (
	"container/heap"
	"errors"
)

// Step is one tile of movement.
type Step struct{ DX, DY int } // exactly one of DX/DY is -1 or +1

var (
	StepUp    = Step{0, -1}
	StepDown  = Step{0, 1}
	StepLeft  = Step{-1, 0}
	StepRight = Step{1, 0}
)

// String returns the step's direction name.
func (s Step) String() string {
	switch s {
	case StepUp:
		return "up"
	case StepDown:
		return "down"
	case StepLeft:
		return "left"
	case StepRight:
		return "right"
	default:
		return "invalid"
	}
}

// ErrNoPath is returned when no route exists.
var ErrNoPath = errors.New("world: no path")

var stepDirs = []Step{StepUp, StepDown, StepLeft, StepRight}

// astarNode is one entry of the A* open set: the tile (x,y) with its
// g-cost (steps from the start) and f-cost (g + Manhattan estimate).
type astarNode struct {
	x, y int
	g, f int
}

// astarOpen is the open set: a min-heap on f, tie-broken toward the node
// closest to the goal (larger g) so the search prefers straight progress.
type astarOpen []*astarNode

func (o astarOpen) Len() int { return len(o) }

func (o astarOpen) Less(i, j int) bool {
	if o[i].f != o[j].f {
		return o[i].f < o[j].f
	}
	return o[i].g > o[j].g
}

func (o astarOpen) Swap(i, j int)       { o[i], o[j] = o[j], o[i] }
func (o *astarOpen) Push(v any)         { *o = append(*o, v.(*astarNode)) }
func (o *astarOpen) Pop() any {
	old := *o
	n := len(old)
	v := old[n-1]
	old[n-1] = nil
	*o = old[:n-1]
	return v
}

// FindPath returns the sequence of steps from (sx,sy) to (dx,dy), avoiding
// tiles that are not walkable and any tile in blocked. blocked may be nil;
// it carries dynamic obstacles such as NPC positions.
func FindPath(g *Grid, sx, sy, dx, dy int, blocked map[[2]int]bool) ([]Step, error) {
	if !g.InBounds(dx, dy) || !g.Walkable(dx, dy) || blocked[[2]int{dx, dy}] {
		return nil, ErrNoPath
	}
	if sx == dx && sy == dy {
		return []Step{}, nil
	}
	if !g.InBounds(sx, sy) || !g.Walkable(sx, sy) || blocked[[2]int{sx, sy}] {
		return nil, ErrNoPath
	}

	start := [2]int{sx, sy}
	cost := map[[2]int]int{start: 0}
	parent := map[[2]int][2]int{}
	closed := map[[2]int]bool{}

	open := &astarOpen{}
	heap.Push(open, &astarNode{x: sx, y: sy, f: manhattan(sx, sy, dx, dy)})

	for open.Len() > 0 {
		cur := heap.Pop(open).(*astarNode)
		c := [2]int{cur.x, cur.y}
		if closed[c] {
			continue
		}
		closed[c] = true
		if cur.x == dx && cur.y == dy {
			return stepsBetween(parent, start, c), nil
		}
		for _, s := range stepDirs {
			n := [2]int{cur.x + s.DX, cur.y + s.DY}
			if closed[n] || blocked[n] || !g.Walkable(n[0], n[1]) {
				continue
			}
			ng := cur.g + 1
			if old, seen := cost[n]; seen && ng >= old {
				continue
			}
			cost[n] = ng
			parent[n] = c
			heap.Push(open, &astarNode{x: n[0], y: n[1], g: ng, f: ng + manhattan(n[0], n[1], dx, dy)})
		}
	}
	return nil, ErrNoPath
}

// manhattan is the 4-way movement distance estimate.
func manhattan(x, y, dx, dy int) int {
	return iabs(x-dx) + iabs(y-dy)
}

func iabs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// stepsBetween walks the parent chain from goal back to start and returns
// the steps in forward order.
func stepsBetween(parent map[[2]int][2]int, start, goal [2]int) []Step {
	var steps []Step
	for cur := goal; cur != start; {
		prev := parent[cur]
		steps = append(steps, Step{DX: cur[0] - prev[0], DY: cur[1] - prev[1]})
		cur = prev
	}
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	return steps
}
