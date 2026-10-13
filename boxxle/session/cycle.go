package session

import (
	"sort"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/decision"
	"github.com/maestroi/pokepilot/boxxle/solver"
)

// chooseAvoidingCycle asks choose for the next push, unless that push would
// restore a crate layout already visited on this puzzle. A restored layout is
// a stall: the greedy score and a model that agrees with it will undo the
// push forever (a nearer goal the solver cannot actually use). The rest of
// the puzzle is then played by the deterministic solver, and every selection
// taken on that path is marked Fallback.
//
// stuck is nil until the first stall. seen records crate layouts this puzzle
// has already occupied; the caller clears both when the next puzzle starts.
func chooseAvoidingCycle(
	state boxxle.State,
	choose func(boxxle.State) (decision.Selection, error),
	seen map[string]bool,
	stuck *SolverChooser,
) (decision.Selection, *SolverChooser, error) {
	if stuck != nil {
		sel, err := stuck.Choose(state)
		if err != nil {
			return decision.Selection{}, stuck, err
		}
		sel.Fallback = true
		return sel, stuck, nil
	}
	sel, err := choose(state)
	if err != nil {
		return decision.Selection{}, nil, err
	}
	if seen != nil && pushRestoresLayout(seen, state, sel.Plan.Push) {
		next := NewSolverChooser()
		alt, altErr := next.Choose(state)
		if altErr != nil {
			seen[layoutKey(state)] = true
			return sel, nil, nil
		}
		alt.Fallback = true
		alt.Request = sel.Request
		alt.Response = sel.Response
		alt.Deterministic = sel.Deterministic
		sel = alt
		stuck = next
	}
	if seen != nil {
		seen[layoutKey(state)] = true
	}
	return sel, stuck, nil
}

// pushRestoresLayout reports whether push lands on a crate layout seen has
// already recorded. A push that the board rejects is not a stall; the caller
// still has to execute it and observe the failure.
func pushRestoresLayout(seen map[string]bool, state boxxle.State, push boxxle.LegalPush) bool {
	next, err := solver.Apply(state, solver.Push{
		CrateFrom:  push.Crate,
		CrateTo:    push.CrateTo,
		Dir:        push.Dir,
		PlayerFrom: push.PlayerFrom,
	})
	if err != nil {
		return false
	}
	return seen[layoutKey(next)]
}

// layoutKey is the crate positions, independent of where the player stands.
// Two boards with the same crates are the same puzzle state for stall
// detection: the player can always walk between them when the push is legal.
func layoutKey(state boxxle.State) string {
	crates := append([]boxxle.Pos(nil), state.Crates...)
	sort.Slice(crates, func(i, j int) bool {
		if crates[i].Y != crates[j].Y {
			return crates[i].Y < crates[j].Y
		}
		return crates[i].X < crates[j].X
	})
	var b strings.Builder
	for i, c := range crates {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(strconv.Itoa(c.X))
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(c.Y))
	}
	return b.String()
}
