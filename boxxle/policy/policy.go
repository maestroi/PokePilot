// Package policy chooses semantic Boxxle (Sokoban) crate pushes without
// driving emulator input.
//
// The policy is the deterministic layer between the decoded board and the
// executor. It enumerates legal pushes from the board geometry, rejects any
// push that deadlocks the puzzle (a corner, a freeze, or any push that leaves
// a solvable board unsolvable), scores the remainder with a stable heuristic,
// and returns the best push. A higher-level planner (an LLM or a scripted
// objective) may override the choice, but it may only select from the
// candidates this package produces: legality and the deadlock guard remain
// deterministic policy responsibilities.
package policy

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/solver"
)

// ErrNoPush is returned when the board has no legal, non-deadlocking push.
var ErrNoPush = errors.New("boxxle policy: no legal push")

// Candidate is one legal crate push with its deterministic score.
type Candidate struct {
	Push       boxxle.LegalPush `json:"push"`
	Score      int              `json:"score"`
	OntoGoal   bool             `json:"onto_goal"`
	DeltaGoal  int              `json:"delta_goal"` // reduction in distance to nearest goal
	PlayerDist int              `json:"player_dist"`
}

// Decision is the policy's output: the chosen candidate plus context.
type Decision struct {
	Candidate  Candidate `json:"candidate"`
	Considered int       `json:"considered"`
	Rejected   int       `json:"rejected"` // pushes rejected by the deadlock guard
}

// Choose selects the best crate push for a decoded board. It returns a typed
// error when the board is not a usable puzzle or when no legal,
// non-deadlocking push exists. The choice is deterministic: the same board
// always yields the same candidate.
func Choose(state boxxle.State) (Decision, error) {
	board, err := boxxle.NewBoard(state)
	if err != nil {
		return Decision{}, err
	}
	if state.Solved {
		return Decision{}, fmt.Errorf("%w: board is already solved", ErrNoPush)
	}
	pushes, rejected := actionable(state, board)
	if len(pushes) == 0 {
		return Decision{}, fmt.Errorf("%w: no legal pushes", ErrNoPush)
	}

	best := Candidate{}
	haveBest := false
	for _, p := range pushes {
		c := score(board, p)
		if !haveBest || c.Score > best.Score || (c.Score == best.Score && c.PlayerDist < best.PlayerDist) {
			best = c
			haveBest = true
		}
	}
	if !haveBest {
		return Decision{}, fmt.Errorf("%w: all %d legal pushes deadlock", ErrNoPush, len(pushes))
	}
	return Decision{
		Candidate:  best,
		Considered: len(pushes),
		Rejected:   rejected,
	}, nil
}

// Candidates returns every legal, non-deadlocking push with its score, in
// deterministic order. A bounded decision engine (e.g. an LLM planner) must
// select only from this set; it may not invent a push the geometry did not
// enumerate.
func Candidates(state boxxle.State) ([]Candidate, error) {
	board, err := boxxle.NewBoard(state)
	if err != nil {
		return nil, err
	}
	pushes, _ := actionable(state, board)
	out := make([]Candidate, 0, len(pushes))
	for _, p := range pushes {
		out = append(out, score(board, p))
	}
	return out, nil
}

// actionable is the set of pushes a planner may select.
//
// PushDeadlock (corners and freezes) is rejected outright. When the board
// still has a solution, a push that leaves it unsolvable is rejected too:
// legal and not a local deadlock, but it seals the room. Level 1's second
// crate can be pushed sideways into the corridor the first crate just left;
// the freeze test catches that case locally, and the solver filter is the
// belt-and-suspenders for seals that are not freezes. The model and the
// greedy score both prefer the sealing push because it shortens the walk to
// a goal. Offering it is how a solvable puzzle gets ruined.
//
// A board the solver cannot solve keeps the locally-filtered set, so a puzzle
// that is already lost still reports "no legal pushes" instead of inventing a
// recovery the search does not have.
func actionable(state boxxle.State, board boxxle.Board) ([]boxxle.LegalPush, int) {
	var open []boxxle.LegalPush
	rejected := 0
	for _, p := range board.LegalPushes() {
		if board.PushDeadlock(p) {
			rejected++
			continue
		}
		open = append(open, p)
	}
	if len(open) == 0 {
		return nil, rejected
	}
	if ok, err := solver.Solvable(state); err != nil || !ok {
		return open, rejected
	}
	kept := make([]boxxle.LegalPush, 0, len(open))
	for _, p := range open {
		if pushPreservesSolution(state, p) {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return open, rejected
	}
	return kept, rejected + len(open) - len(kept)
}

// pushPreservesSolution reports whether applying p leaves a board the
// deterministic solver can still finish.
func pushPreservesSolution(state boxxle.State, p boxxle.LegalPush) bool {
	next, err := solver.Apply(state, solver.Push{
		CrateFrom:  p.Crate,
		CrateTo:    p.CrateTo,
		Dir:        p.Dir,
		PlayerFrom: p.PlayerFrom,
	})
	if err != nil {
		return false
	}
	ok, err := solver.Solvable(next)
	return err == nil && ok
}

// score applies the deterministic Sokoban heuristic to a legal push.
//
// The heuristic rewards:
//   - moving a crate onto a goal (+1000),
//   - reducing the crate's Manhattan distance to its nearest goal (+10 per
//     cell),
//
// and penalizes longer player walks (−1 per step) so that, between equally
// goal-progressing pushes, the cheaper one wins.
func score(board boxxle.Board, p boxxle.LegalPush) Candidate {
	c := Candidate{Push: p}

	// Distance from the crate's new cell to the nearest goal.
	after := nearestGoalDist(board, p.CrateTo)
	before := nearestGoalDist(board, p.Crate)
	c.DeltaGoal = before - after

	// Reward progress, not state: a crate that is already on a goal gets no
	// bonus for moving to another goal. Without this, a crate parked on one
	// goal oscillates between adjacent goals forever, each move scoring the
	// full bonus, and the policy never pushes the remaining crates.
	if board.IsGoal(p.CrateTo) && !board.IsGoal(p.Crate) {
		c.OntoGoal = true
		c.Score += 1000
	}
	c.Score += 10 * c.DeltaGoal

	// Player walk cost: steps from the current player position to the
	// pre-push square.
	if path, ok := board.PathTo(*board.State().Player, p.PlayerFrom); ok {
		c.PlayerDist = len(path)
	}
	c.Score -= c.PlayerDist

	return c
}

// nearestGoalDist returns the Manhattan distance from p to the nearest goal,
// or a large penalty when the board has no goals.
func nearestGoalDist(board boxxle.Board, p boxxle.Pos) int {
	state := board.State()
	if len(state.Goals) == 0 {
		return 10000
	}
	best := 1 << 30
	for _, g := range state.Goals {
		d := manhattan(p, g)
		if d < best {
			best = d
		}
	}
	return best
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
