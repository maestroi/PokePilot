package agent

import (
	"errors"
	"fmt"
	"io"

	"github.com/maestroi/pokepilot/emu"
)

// Stop says why a run ended.
type Stop uint8

const (
	StopDone   Stop = iota // the planner reported ErrDone
	StopStuck              // no progress for too many rounds
	StopBudget             // the round or frame budget ran out
	StopError              // an objective failed
)

// Result is the outcome of a run.
type Result struct {
	Stop      Stop
	Rounds    int
	Completed []Objective
	Err       error // set when Stop is StopError
	Final     Observation
}

// defaultStuckAfter is the StuckAfter used when Budget leaves it zero.
// Small on purpose: a run that is not stuck will change the map, the
// position, the party, or the events within a few rounds.
const defaultStuckAfter = 3

// Budget bounds a run. MaxRounds and MaxFrames are both required; a zero
// budget is an error, not "unlimited". An unbounded loop against an
// emulator is how a run silently eats 45 minutes.
type Budget struct {
	MaxRounds int
	MaxFrames int
	// StuckAfter is how many consecutive objectives may leave the
	// observation unchanged before the run stops with StopStuck.
	// Zero means defaultStuckAfter.
	StuckAfter int
	// Log receives one line per round. Nil means no logging.
	Log io.Writer
	// Cancel, when closed, stops Run before the next round's objective
	// starts. Nil means never cancelled — the zero value of Budget keeps
	// every existing caller's behavior unchanged. Checked between rounds
	// only: an objective already in flight always finishes.
	Cancel <-chan struct{}
}

// Run drives observe -> plan -> execute until the planner is done or a
// budget is exhausted. It never retries a failed objective and never
// continues past one: a planner reasoning from a state it thinks it
// reached is worse than stopping.
func Run(m *emu.Emu, romData []byte, p Planner, offered []Objective, budget Budget) Result {
	if budget.MaxRounds <= 0 || budget.MaxFrames <= 0 {
		return Result{
			Stop: StopError,
			Err:  errors.New("agent: Run: a zero budget is not unlimited; set MaxRounds and MaxFrames"),
		}
	}
	stuckAfter := budget.StuckAfter
	if stuckAfter <= 0 {
		stuckAfter = defaultStuckAfter
	}

	res := Result{Completed: []Objective{}}
	// A run cancelled before round 1 must stop before it touches the
	// emulator at all: there is no observation to report, and callers
	// may pass a nil emu for exactly that case.
	select {
	case <-budget.Cancel:
		return Result{Stop: StopBudget, Rounds: 0}
	default:
	}
	startFrame := m.FrameCount()
	last := Observe(m)
	stuck := 0

	for round := 1; ; round++ {
		select {
		case <-budget.Cancel:
			return Result{Stop: StopBudget, Rounds: round - 1}
		default:
		}

		if round > budget.MaxRounds {
			res.Stop = StopBudget
			break
		}

		// Rebuilt every round: what is possible depends on where the
		// player is and what they already have. A nil offered list means
		// the planner does not use a menu (ScriptedPlanner), so there is
		// nothing to narrow and nothing to run out of.
		now := offered
		if len(offered) > 0 {
			if now = Offer(last, offered); len(now) == 0 {
				res.Stop = StopError
				res.Err = errors.New("agent: Run: nothing is possible from here")
				break
			}
		}

		obj, err := p.Next(last, now)
		if errors.Is(err, ErrDone) {
			res.Stop = StopDone
			break
		}
		if err != nil {
			res.Stop = StopError
			res.Err = err
			break
		}

		before := last
		if err := Execute(m, romData, obj); err != nil {
			res.Stop = StopError
			res.Err = err
			last = Observe(m)
			logRound(budget.Log, round, obj, last)
			break
		}
		last = Observe(m)
		res.Rounds = round
		res.Completed = append(res.Completed, obj)
		logRound(budget.Log, round, obj, last)

		if sameProgress(before, last) {
			stuck++
		} else {
			stuck = 0
		}
		if stuck >= stuckAfter {
			res.Stop = StopStuck
			break
		}
		if m.FrameCount()-startFrame >= uint64(budget.MaxFrames) {
			res.Stop = StopBudget
			break
		}
	}

	res.Final = last
	return res
}

// sameProgress reports whether two observations are identical on the fields
// that count as progress: the map, the position, the party count, and the
// event list. Everything else (facing, money, HP) may drift without saying
// the run is making headway.
func sameProgress(a, b Observation) bool {
	if a.Map != b.Map || a.X != b.X || a.Y != b.Y || a.PartyCount != b.PartyCount {
		return false
	}
	if len(a.Events) != len(b.Events) {
		return false
	}
	for i := range a.Events {
		if a.Events[i] != b.Events[i] {
			return false
		}
	}
	return true
}

// logRound writes the one per-round line that makes an overnight run
// diagnosable in the morning: the round number, what was attempted, and
// where the player ended up.
func logRound(w io.Writer, round int, o Objective, after Observation) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "round %d: %s -> map %02x at (%d,%d)\n", round, o, after.Map, after.X, after.Y)
}
