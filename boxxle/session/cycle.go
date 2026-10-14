package session

import (
	"sort"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/decision"
	"github.com/maestroi/pokepilot/boxxle/solver"
)

// chooseAvoidingCycle asks choose for the next push. seen is the crate
// layouts this puzzle has already occupied, and choose receives it so a model
// can solve the room from the pushes that do not undo one. A chooser that
// ignores seen and restores a layout anyway is stuck: the nearer goal is a
// dead end the score will undo forever. The rest of that puzzle is played by
// the deterministic solver, and every selection on that path is marked Fallback.
//
// stuck is nil until that happens. The caller clears seen and stuck when the
// next puzzle starts.
func chooseAvoidingCycle(
	state boxxle.State,
	choose func(boxxle.State, map[string]bool) (decision.Selection, error),
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
	sel, err := choose(state, seen)
	if err != nil {
		return decision.Selection{}, nil, err
	}
	if seen != nil && PushRestoresLayout(seen, state, sel.Plan.Push) {
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

// PushRestoresLayout reports whether push lands on a crate layout seen has
// already recorded. A push that the board rejects is not a stall; the caller
// still has to execute it and observe the failure.
func PushRestoresLayout(seen map[string]bool, state boxxle.State, push boxxle.LegalPush) bool {
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
