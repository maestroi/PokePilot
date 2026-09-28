// Package solver is a deterministic, ROM-free Sokoban solver for decoded
// Boxxle boards, plus the checked-in fixture set it benchmarks against.
//
// The search is A* over push states: a state is the set of crate positions
// plus the player's reachable region (the cells the player can walk to), not
// the player's exact cell. Two push states with the same crates and the same
// reachable region admit exactly the same future pushes, so canonicalizing
// the region deduplicates walking steps without losing any solution.
//
// The search is deterministic: successors iterate crates in sorted order and
// directions in a fixed order, and the open list breaks ties on a stable
// state key. The same board always yields the same solution.
//
// Pruning is cheap and admissible (no research-grade Sokoban heuristics):
//   - static dead squares (corners): a crate pushed onto one can never reach
//     a goal, so such pushes are never generated;
//   - crate-goal reachability: a pull-BFS from the goals marks the cells a
//     crate can ever occupy on its way to a goal, ignoring other crates. The
//     test is necessary, not sufficient, so it never prunes a winning line.
package solver

import (
	"container/heap"
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/boxxle"
)

// Puzzle is a named, ROM-free Sokoban board.
type Puzzle struct {
	Name  string
	State boxxle.State
}

// Push is one crate push in a solution. Walk is the number of player steps
// taken to reach PlayerFrom before the push, counted during reconstruction.
type Push struct {
	CrateFrom  boxxle.Pos
	CrateTo    boxxle.Pos
	Dir        boxxle.Direction
	PlayerFrom boxxle.Pos
	Walk       int
}

// Solution is a complete push sequence that solves the puzzle.
type Solution struct {
	Pushes []Push
}

// PushCount is the number of crate pushes in the solution.
func (s Solution) PushCount() int { return len(s.Pushes) }

// WalkCount is the total number of player walking steps in the solution.
func (s Solution) WalkCount() int {
	n := 0
	for _, p := range s.Pushes {
		n += p.Walk
	}
	return n
}

// Solve finds a minimum-push solution for a decoded board, or reports the
// puzzle unsolvable. It is ROM-free: it reasons only over the typed State,
// so it doubles as the oracle/reference for benchmarking model plans.
//
// The second return value is the solvability answer: true with a Solution
// when solved, false with an empty Solution when the search space is
// exhausted (or the start position is already dead). An error is reserved
// for a board that is not a usable puzzle.
func Solve(state boxxle.State) (Solution, bool, error) {
	board, err := boxxle.NewBoard(state)
	if err != nil {
		return Solution{}, false, err
	}
	if len(state.Crates) == 0 {
		return Solution{Pushes: []Push{}}, true, nil
	}
	if len(state.Goals) == 0 {
		return Solution{}, false, nil
	}

	dead := board.DeadSquares()
	goalReach := crateGoalCells(board)

	start := newPushState(state)
	if deadlocked(start.crates, dead, goalReach) {
		return Solution{}, false, nil
	}

	parent := map[string]*searchNode{startKey(start): {st: start}}
	open := &openList{}
	heap.Init(open)
	heap.Push(open, &openNode{st: start, key: startKey(start), g: 0, f: heuristic(start.crates, state.Goals)})

	for open.Len() > 0 {
		cur := heap.Pop(open).(*openNode)
		if solved(cur.st.crates, state.Goals) {
			return reconstruct(parent, cur.key, state), true, nil
		}
		for _, next := range successors(cur.st, board, dead, goalReach) {
			if _, seen := parent[nextKey(next.st)]; seen {
				continue
			}
			parent[nextKey(next.st)] = &searchNode{st: next.st, parentKey: cur.key, push: next.push}
			g := cur.g + 1
			heap.Push(open, &openNode{st: next.st, key: nextKey(next.st), g: g, f: g + heuristic(next.st.crates, state.Goals)})
		}
	}
	return Solution{}, false, nil
}

// SolvePuzzle is Solve for a named fixture.
func SolvePuzzle(p Puzzle) (Solution, bool, error) {
	return Solve(p.State)
}

// Apply performs one push on a board state and returns the resulting state.
// It validates the push against the state (crate present, target free,
// pre-push square walkable and reachable), so a replayed solution that does
// not match the board fails loudly instead of silently corrupting state.
func Apply(state boxxle.State, p Push) (boxxle.State, error) {
	board, err := boxxle.NewBoard(state)
	if err != nil {
		return state, err
	}
	lp, ok := board.FindPush(p.CrateFrom, p.Dir)
	if !ok {
		return state, fmt.Errorf("boxxle solver: push of crate %v %s is not legal in this state", p.CrateFrom, p.Dir)
	}
	if lp.CrateTo != p.CrateTo {
		return state, fmt.Errorf("boxxle solver: push of crate %v %s lands on %v, not %v", p.CrateFrom, p.Dir, lp.CrateTo, p.CrateTo)
	}
	next := state
	next.Crates = make([]boxxle.Pos, 0, len(state.Crates))
	for _, c := range state.Crates {
		if c == p.CrateFrom {
			c = p.CrateTo
		}
		next.Crates = append(next.Crates, c)
	}
	next.Player = &lp.Crate
	next.Solved = allOnGoals(next.Crates, next.Goals)
	return next, nil
}

// Replay applies every push of a solution to the start state and returns the
// final state. It is the positive postcondition check for a solution: a
// solution is valid only when the replayed board reports Solved.
func Replay(state boxxle.State, sol Solution) (boxxle.State, error) {
	cur := state
	for _, p := range sol.Pushes {
		var err error
		cur, err = Apply(cur, p)
		if err != nil {
			return cur, err
		}
	}
	return cur, nil
}

// pushState is one node of the search: crate positions (sorted, so the key is
// canonical) and the player's reachable region (sorted cell indices).
type pushState struct {
	crates []boxxle.Pos
	region []int
}

type searchNode struct {
	st        pushState
	parentKey string
	push      Push
}

type openNode struct {
	st  pushState
	key string
	g   int
	f   int
}

type openList []*openNode

func (l openList) Len() int { return len(l) }
func (l openList) Less(i, j int) bool {
	a, b := l[i], l[j]
	if a.f != b.f {
		return a.f < b.f
	}
	if a.g != b.g {
		return a.g < b.g
	}
	return a.key < b.key
}
func (l openList) Swap(i, j int)       { l[i], l[j] = l[j], l[i] }
func (l *openList) Push(x interface{}) { *l = append(*l, x.(*openNode)) }
func (l *openList) Pop() interface{} {
	old := *l
	n := len(old)
	x := old[n-1]
	*l = old[:n-1]
	return x
}

func newPushState(state boxxle.State) pushState {
	crates := append([]boxxle.Pos(nil), state.Crates...)
	sort.Slice(crates, func(i, j int) bool {
		if crates[i].Y != crates[j].Y {
			return crates[i].Y < crates[j].Y
		}
		return crates[i].X < crates[j].X
	})
	region := playerRegion(state, crateSet(crates), *state.Player)
	return pushState{crates: crates, region: region}
}

func startKey(st pushState) string { return stateKey(st.crates, st.region) }
func nextKey(st pushState) string  { return stateKey(st.crates, st.region) }

func stateKey(crates []boxxle.Pos, region []int) string {
	var b strings.Builder
	for _, c := range crates {
		b.WriteByte('C')
		fmt.Fprintf(&b, "%d,%d;", c.X, c.Y)
	}
	for _, i := range region {
		b.WriteByte('R')
		fmt.Fprintf(&b, "%d;", i)
	}
	return b.String()
}

func crateSet(crates []boxxle.Pos) map[boxxle.Pos]bool {
	set := make(map[boxxle.Pos]bool, len(crates))
	for _, c := range crates {
		set[c] = true
	}
	return set
}

func cellIndex(state boxxle.State, p boxxle.Pos) int {
	return p.Y*state.Width + p.X
}

// playerRegion returns the sorted cell indices the player can stand on from
// start, treating walls and the given crate set as solid.
func playerRegion(state boxxle.State, crates map[boxxle.Pos]bool, start boxxle.Pos) []int {
	solid := func(p boxxle.Pos) bool {
		if p.X < 0 || p.X >= state.Width || p.Y < 0 || p.Y >= state.Height {
			return true
		}
		return crates[p] || wallSet(state)[p]
	}
	seen := map[int]bool{cellIndex(state, start): true}
	queue := []boxxle.Pos{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for d := boxxle.DirUp; d <= boxxle.DirRight; d++ {
			delta := d.Offset()
			nxt := boxxle.Pos{X: cur.X + delta.X, Y: cur.Y + delta.Y}
			i := cellIndex(state, nxt)
			if seen[i] || solid(nxt) {
				continue
			}
			seen[i] = true
			queue = append(queue, nxt)
		}
	}
	out := make([]int, 0, len(seen))
	for i := range seen {
		out = append(out, i)
	}
	sort.Ints(out)
	return out
}

// wallSet returns the wall set for a state. The fixture boards are small, so
// rebuilding the set per region computation is cheap enough for this slice.
func wallSet(state boxxle.State) map[boxxle.Pos]bool {
	set := make(map[boxxle.Pos]bool, len(state.Walls))
	for _, w := range state.Walls {
		set[w] = true
	}
	return set
}

type successor struct {
	st   pushState
	push Push
}

// successors enumerates every pruned push from st, in deterministic order:
// crates in sorted order, directions up/down/left/right.
func successors(st pushState, board boxxle.Board, dead, goalReach map[boxxle.Pos]bool) []successor {
	state := board.State()
	var out []successor
	crates := crateSet(st.crates)
	regionSet := make(map[int]bool, len(st.region))
	for _, i := range st.region {
		regionSet[i] = true
	}

	for _, crate := range st.crates {
		for d := boxxle.DirUp; d <= boxxle.DirRight; d++ {
			delta := d.Offset()
			crateTo := boxxle.Pos{X: crate.X + delta.X, Y: crate.Y + delta.Y}
			playerFrom := boxxle.Pos{X: crate.X - delta.X, Y: crate.Y - delta.Y}

			if !inBounds(state, crateTo) || board.IsWall(crateTo) || crates[crateTo] {
				continue
			}
			if !inBounds(state, playerFrom) || board.IsWall(playerFrom) || crates[playerFrom] {
				continue
			}
			if !regionSet[cellIndex(state, playerFrom)] {
				continue
			}
			// Deadlock pruning: never push a crate onto a corner dead
			// square or onto a cell no goal is pull-reachable from.
			if dead[crateTo] || !goalReach[crateTo] {
				continue
			}

			nextCrates := replaceCrate(st.crates, crate, crateTo)
			nextCratesSet := crateSet(nextCrates)
			next := pushState{
				crates: nextCrates,
				region: playerRegion(state, nextCratesSet, crate),
			}
			out = append(out, successor{
				st: next,
				push: Push{
					CrateFrom:  crate,
					CrateTo:    crateTo,
					Dir:        d,
					PlayerFrom: playerFrom,
				},
			})
		}
	}
	return out
}

func inBounds(state boxxle.State, p boxxle.Pos) bool {
	return p.X >= 0 && p.X < state.Width && p.Y >= 0 && p.Y < state.Height
}

func replaceCrate(crates []boxxle.Pos, from, to boxxle.Pos) []boxxle.Pos {
	out := make([]boxxle.Pos, len(crates))
	for i, c := range crates {
		if c == from {
			out[i] = to
		} else {
			out[i] = c
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

// crateGoalCells marks the cells a crate can ever occupy on its way to some
// goal, computed by a pull-BFS from the goals. A crate at `from` pushed in
// direction d lands on `cur`; reversing that, `from` is pull-reachable when
// `from` and the player's standing square behind it are both open. Other
// crates are ignored, so the marking is a superset of the true reachable set:
// it prunes only crates that can provably never reach a goal.
func crateGoalCells(board boxxle.Board) map[boxxle.Pos]bool {
	state := board.State()
	reach := map[boxxle.Pos]bool{}
	var queue []boxxle.Pos
	for _, g := range state.Goals {
		if !reach[g] {
			reach[g] = true
			queue = append(queue, g)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for d := boxxle.DirUp; d <= boxxle.DirRight; d++ {
			delta := d.Offset()
			from := boxxle.Pos{X: cur.X - delta.X, Y: cur.Y - delta.Y}
			behind := boxxle.Pos{X: from.X - delta.X, Y: from.Y - delta.Y}
			if !inBounds(state, from) || board.IsWall(from) {
				continue
			}
			if !inBounds(state, behind) || board.IsWall(behind) {
				continue
			}
			if !reach[from] {
				reach[from] = true
				queue = append(queue, from)
			}
		}
	}
	return reach
}

func deadlocked(crates []boxxle.Pos, dead, goalReach map[boxxle.Pos]bool) bool {
	for _, c := range crates {
		if dead[c] || !goalReach[c] {
			return true
		}
	}
	return false
}

func solved(crates []boxxle.Pos, goals []boxxle.Pos) bool {
	return allOnGoals(crates, goals)
}

// heuristic is the sum over crates of the Manhattan distance to the nearest
// goal. It is admissible and consistent for a push cost of 1, so A* with it
// returns a minimum-push solution.
func heuristic(crates, goals []boxxle.Pos) int {
	h := 0
	for _, c := range crates {
		best := 1 << 30
		for _, g := range goals {
			d := manhattan(c, g)
			if d < best {
				best = d
			}
		}
		h += best
	}
	return h
}

func manhattan(a, b boxxle.Pos) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

// reconstruct walks the parent chain back to the start, then replays the
// pushes to count the player walking steps each one requires.
func reconstruct(parent map[string]*searchNode, goalKey string, state boxxle.State) Solution {
	var pushes []Push
	for k := goalKey; ; {
		n := parent[k]
		if n.parentKey == "" {
			break
		}
		pushes = append(pushes, n.push)
		k = n.parentKey
	}
	for i, j := 0, len(pushes)-1; i < j; i, j = i+1, j-1 {
		pushes[i], pushes[j] = pushes[j], pushes[i]
	}

	player := *state.Player
	for i, p := range pushes {
		board, err := boxxle.NewBoard(state)
		if err == nil {
			if path, ok := board.PathTo(player, p.PlayerFrom); ok {
				pushes[i].Walk = len(path)
			}
		}
		state.Crates = replaceCrate(state.Crates, p.CrateFrom, p.CrateTo)
		player = p.CrateFrom
	}
	return Solution{Pushes: pushes}
}
