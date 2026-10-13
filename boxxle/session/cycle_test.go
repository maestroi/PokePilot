package session

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/decision"
	"github.com/maestroi/pokepilot/boxxle/policy"
	"github.com/maestroi/pokepilot/boxxle/solver"
)

// The live endless run run-3b84ujtv6em4j2zkkqmajgfzkp sat on this board and
// pushed one crate left, then right, hundreds of times. The nearer goal is a
// dead end: moving toward it stays solvable only because the push can be
// undone, and both the greedy score and the model then pick the undo.
func TestGreedyPushCycleHandsThePuzzleToTheSolver(t *testing.T) {
	state := renderState(t, []string{
		"##########",
		"#++@.....#",
		"#*+.$.#..#",
		"#..#.##.##",
		"#.$.....##",
		"#####.#.##",
		"###.$...##",
		"###.....##",
		"##########",
	})
	calls := 0
	choose := func(s boxxle.State) (decision.Selection, error) {
		calls++
		d, err := policy.Choose(s)
		if err != nil {
			return decision.Selection{}, err
		}
		plan := decision.Plan{Push: d.Candidate.Push}
		return decision.Selection{Plan: plan, Deterministic: plan}, nil
	}

	seen := map[string]bool{}
	var stuck *SolverChooser
	var fallbacks int
	for step := 0; step < 40; step++ {
		if state.Solved {
			break
		}
		sel, next, err := chooseAvoidingCycle(state, choose, seen, stuck)
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		stuck = next
		if sel.Fallback {
			fallbacks++
		}
		nextState, err := solver.Apply(state, solver.Push{
			CrateFrom:  sel.Plan.Push.Crate,
			CrateTo:    sel.Plan.Push.CrateTo,
			Dir:        sel.Plan.Push.Dir,
			PlayerFrom: sel.Plan.Push.PlayerFrom,
		})
		if err != nil {
			t.Fatalf("step %d apply %v %s: %v\n%s", step, sel.Plan.Push.Crate, sel.Plan.Push.Dir, err, strings.Join(boxxle.RenderBoard(state), "\n"))
		}
		state = nextState
	}
	if !state.Solved {
		t.Fatalf("cycle was not broken; board after 40 pushes:\n%s", strings.Join(boxxle.RenderBoard(state), "\n"))
	}
	if fallbacks == 0 {
		t.Fatal("solver fallback did not take over")
	}
	if calls >= 40 {
		t.Fatalf("model/policy was consulted %d times; the cycle should hand the puzzle to the solver", calls)
	}
}

func renderState(t *testing.T, rows []string) boxxle.State {
	t.Helper()
	h := len(rows)
	w := 0
	for _, row := range rows {
		if len(row) > w {
			w = len(row)
		}
	}
	var walls, goals, crates []boxxle.Pos
	var player *boxxle.Pos
	for y, row := range rows {
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case '#':
				walls = append(walls, boxxle.Pos{X: x, Y: y})
			case '@':
				player = &boxxle.Pos{X: x, Y: y}
			case '$':
				crates = append(crates, boxxle.Pos{X: x, Y: y})
			case '*':
				crates = append(crates, boxxle.Pos{X: x, Y: y})
				goals = append(goals, boxxle.Pos{X: x, Y: y})
			case '+':
				goals = append(goals, boxxle.Pos{X: x, Y: y})
			}
		}
	}
	return boxxle.State{
		Screen: boxxle.ScreenPuzzle,
		Width:  w, Height: h,
		Walls: walls, Goals: goals, Crates: crates, Player: player,
	}
}
