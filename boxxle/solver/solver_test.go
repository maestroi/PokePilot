package solver

import (
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/boxxle"
)

// expectedFixtures is the hand-verified ground truth for the checked-in
// fixture set: which puzzles are solvable and the minimum push count of the
// solvable ones. The solver is A* with an admissible heuristic, so the
// minimum count is the exact optimum — asserting it pins both optimality and
// determinism.
var expectedFixtures = map[string]struct {
	solvable bool
	pushes   int
}{
	"trivial_1":         {solvable: true, pushes: 1},
	"corner_1":          {solvable: true, pushes: 1},
	"cross_2":           {solvable: true, pushes: 2},
	"two_crate_1":       {solvable: true, pushes: 5},
	"two_crate_2":       {solvable: true, pushes: 3},
	"two_crate_3":       {solvable: true, pushes: 4},
	"corner_unsolvable": {solvable: false},
	"sealed_unsolvable": {solvable: false},
	"no_goal":           {solvable: false},
}

// TestFixturesSolve checks every checked-in fixture against its ground truth
// and replays each solution to the positive postcondition: the replayed
// board must report Solved with every crate on a goal.
func TestFixturesSolve(t *testing.T) {
	puzzles, err := Fixtures()
	if err != nil {
		t.Fatalf("Fixtures: %v", err)
	}
	if len(puzzles) != len(expectedFixtures) {
		t.Fatalf("Fixtures returned %d puzzles, want %d", len(puzzles), len(expectedFixtures))
	}
	for _, p := range puzzles {
		want, ok := expectedFixtures[p.Name]
		if !ok {
			t.Fatalf("fixture %s has no ground-truth entry", p.Name)
		}
		sol, got, err := SolvePuzzle(p)
		if err != nil {
			t.Fatalf("%s: Solve: %v", p.Name, err)
		}
		if got != want.solvable {
			t.Errorf("%s: solvable = %v, want %v", p.Name, got, want.solvable)
			continue
		}
		if !want.solvable {
			if len(sol.Pushes) != 0 {
				t.Errorf("%s: unsolvable puzzle returned %d pushes", p.Name, len(sol.Pushes))
			}
			continue
		}
		if sol.PushCount() != want.pushes {
			t.Errorf("%s: pushes = %d, want %d", p.Name, sol.PushCount(), want.pushes)
		}
		final, err := Replay(p.State, sol)
		if err != nil {
			t.Fatalf("%s: Replay: %v", p.Name, err)
		}
		if !final.Solved {
			t.Errorf("%s: replayed board is not solved (crates %v, goals %v)", p.Name, final.Crates, final.Goals)
		}
	}
}

// TestSolveDeterministic checks that the same board yields the identical
// solution across repeated solves: the search order and tie-breaks are
// deterministic by construction.
func TestSolveDeterministic(t *testing.T) {
	puzzles, err := Fixtures()
	if err != nil {
		t.Fatalf("Fixtures: %v", err)
	}
	for _, p := range puzzles {
		a, okA, err := SolvePuzzle(p)
		if err != nil {
			t.Fatalf("%s: Solve: %v", p.Name, err)
		}
		b, okB, err := SolvePuzzle(p)
		if err != nil {
			t.Fatalf("%s: Solve (second): %v", p.Name, err)
		}
		if okA != okB {
			t.Fatalf("%s: solvability differs across runs: %v vs %v", p.Name, okA, okB)
		}
		if len(a.Pushes) != len(b.Pushes) {
			t.Fatalf("%s: push count differs across runs: %d vs %d", p.Name, len(a.Pushes), len(b.Pushes))
		}
		for i := range a.Pushes {
			if a.Pushes[i] != b.Pushes[i] {
				t.Fatalf("%s: push %d differs across runs: %+v vs %+v", p.Name, i, a.Pushes[i], b.Pushes[i])
			}
		}
	}
}

// TestApplyRejectsIllegalPush checks that replaying a push the board does not
// support fails loudly instead of corrupting state.
func TestApplyRejectsIllegalPush(t *testing.T) {
	p, err := loadFixtureByName(t, "corner_unsolvable")
	if err != nil {
		t.Fatal(err)
	}
	// The crate sits in the top-left corner; pushing it right would need the
	// player standing on the left wall, so the board rejects it.
	_, err = Apply(p.State, Push{
		CrateFrom:  boxxle.Pos{X: 1, Y: 1},
		CrateTo:    boxxle.Pos{X: 2, Y: 1},
		Dir:        boxxle.DirRight,
		PlayerFrom: boxxle.Pos{X: 0, Y: 1},
	})
	if err == nil {
		t.Fatal("Apply accepted a push the board does not support")
	}
}

// TestDeadSquarePruning checks that a board whose only apparent solution
// pushes a crate into a corner is reported unsolvable: the dead-square
// pruning must remove that line from the search, not just reject it late.
func TestDeadSquarePruning(t *testing.T) {
	// A one-cell-wide corridor. The player is trapped at (1,1) behind the
	// crate, so the only possible first push is right: (2,1) -> (3,1). After
	// that the player can never get behind the crate again, so no push can
	// ever bring it back to the goal at (1,1): the search exhausts and the
	// puzzle is unsolvable, with the final (5,1) corner line pruned by the
	// dead-square / goal-reachability checks.
	state := boxxle.State{
		Screen: boxxle.ScreenPuzzle,
		Width:  6,
		Height: 3,
		Walls: []boxxle.Pos{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 3, Y: 0}, {X: 4, Y: 0}, {X: 5, Y: 0},
			{X: 0, Y: 1},
			{X: 0, Y: 2}, {X: 1, Y: 2}, {X: 2, Y: 2}, {X: 3, Y: 2}, {X: 4, Y: 2}, {X: 5, Y: 2},
		},
		Goals:  []boxxle.Pos{{X: 1, Y: 1}},
		Crates: []boxxle.Pos{{X: 2, Y: 1}},
		Player: &boxxle.Pos{X: 1, Y: 1},
	}
	sol, ok, err := Solve(state)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if ok {
		t.Fatalf("expected unsolvable (next push corner-deadlocks), got solution %v", sol.Pushes)
	}
}

// TestGoalReachabilityPruning checks the pull-reachability prune: a goal
// sealed behind walls makes the puzzle unsolvable even when no crate starts
// on a dead square.
func TestGoalReachabilityPruning(t *testing.T) {
	state := boxxle.State{
		Screen: boxxle.ScreenPuzzle,
		Width:  7,
		Height: 7,
		Walls: []boxxle.Pos{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 3, Y: 0}, {X: 4, Y: 0}, {X: 5, Y: 0}, {X: 6, Y: 0},
			{X: 0, Y: 1}, {X: 2, Y: 1}, {X: 3, Y: 1}, {X: 6, Y: 1},
			{X: 0, Y: 2}, {X: 2, Y: 2}, {X: 4, Y: 2}, {X: 6, Y: 2},
			{X: 0, Y: 3}, {X: 2, Y: 3}, {X: 3, Y: 3}, {X: 6, Y: 3},
			{X: 0, Y: 4}, {X: 6, Y: 4},
			{X: 0, Y: 5}, {X: 6, Y: 5},
			{X: 0, Y: 6}, {X: 1, Y: 6}, {X: 2, Y: 6}, {X: 3, Y: 6}, {X: 4, Y: 6}, {X: 5, Y: 6}, {X: 6, Y: 6},
		},
		Goals:  []boxxle.Pos{{X: 3, Y: 2}},
		Crates: []boxxle.Pos{{X: 3, Y: 4}},
		Player: &boxxle.Pos{X: 3, Y: 5},
	}
	_, ok, err := Solve(state)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if ok {
		t.Fatal("expected unsolvable (goal sealed), got solvable")
	}
}

// TestNoCrateSolved checks the vacuous case: a board with no crates is
// trivially solved with an empty solution.
func TestNoCrateSolved(t *testing.T) {
	state := boxxle.State{
		Screen: boxxle.ScreenPuzzle,
		Width:  3,
		Height: 3,
		Walls: []boxxle.Pos{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0},
			{X: 0, Y: 1}, {X: 2, Y: 1},
			{X: 0, Y: 2}, {X: 1, Y: 2}, {X: 2, Y: 2},
		},
		Goals:  []boxxle.Pos{{X: 1, Y: 1}},
		Player: &boxxle.Pos{X: 1, Y: 1},
	}
	sol, ok, err := Solve(state)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if !ok || sol.PushCount() != 0 {
		t.Fatalf("expected trivially solved with 0 pushes, got ok=%v pushes=%d", ok, sol.PushCount())
	}
}

func loadFixtureByName(t *testing.T, name string) (Puzzle, error) {
	t.Helper()
	puzzles, err := Fixtures()
	if err != nil {
		return Puzzle{}, err
	}
	for _, p := range puzzles {
		if p.Name == name {
			return p, nil
		}
	}
	return Puzzle{}, fmt.Errorf("fixture %s not found", name)
}
