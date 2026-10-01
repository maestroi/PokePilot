package session

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/control"
	"github.com/maestroi/pokepilot/boxxle/decision"
	"github.com/maestroi/pokepilot/boxxle/policy"
	"github.com/maestroi/pokepilot/boxxle/solver"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// ErrBootTimeout reports that the cartridge did not reach the puzzle screen
// within the boot budget.
var ErrBootTimeout = errors.New("boxxle session: boot timed out")

// RunOptions configures an autonomous Boxxle puzzle run.
type RunOptions struct {
	// Levels is how many puzzles to solve; zero means one. Every solved puzzle
	// is followed by an advance to the next, so a run that finishes has proven
	// the cartridge moves on, not just that one board was solved.
	Levels int
	// MaxPushes caps the number of executed pushes; zero means no cap.
	MaxPushes int
	// MaxFrames caps the emulated frames after boot; zero means no cap.
	MaxFrames int
	// Cancel, when non-nil, stops the run when closed.
	Cancel <-chan struct{}
	// Choose may replace the deterministic policy selector with another bounded
	// selector, such as a model planner. It returns a decision.Selection; the
	// session executes selection.Plan.Push through the deterministic controller.
	// Nil preserves historical deterministic play.
	Choose func(boxxle.State) (decision.Selection, error)
	// OnDecision, when non-nil, is called after each selection so a caller can
	// record telemetry.
	OnDecision func(decision.Selection)
}

// Result records the outcome of an autonomous Boxxle puzzle run.
type Result struct {
	Reason string `json:"reason"`
	Pushes int    `json:"pushes"`
	// Levels is the number of puzzles solved, each followed by a verified
	// advance to the next puzzle's board.
	Levels        int                 `json:"levels"`
	State         boxxle.State        `json:"state"`
	LastSelection *decision.Selection `json:"last_selection,omitempty"`
	Telemetry     *decision.Snapshot  `json:"telemetry,omitempty"`
	Err           error               `json:"-"`
}

// decoder adapts boxxle.DecodeState to the control.Decoder surface so the
// controller can re-read the board after every step without a profile cast.
type decoder struct{}

func (decoder) DecodeBoxxleState(r game.MemoryReader) (boxxle.State, error) {
	return boxxle.DecodeState(r)
}

// solverChooser is the deterministic selector: it solves the live board once
// and replays that plan, one push per call. Each planned push is re-validated
// against the freshly decoded board (the crate is there, the push is legal from
// where the player now is), and anything that no longer holds drops the plan
// and re-solves, so a run never commits to a stale push. The greedy policy is
// the fallback only for a board the solver declares unsolvable, where it still
// yields a legal, non-deadlocking push.
type solverChooser struct{ plan []solver.Push }

func (c *solverChooser) choose(state boxxle.State) (decision.Selection, error) {
	selection := func(push boxxle.LegalPush) decision.Selection {
		plan := decision.Plan{Push: push}
		return decision.Selection{Plan: plan, Deterministic: plan}
	}
	if board, err := boxxle.NewBoard(state); err == nil && len(c.plan) > 0 {
		next := c.plan[0]
		if push, ok := board.FindPush(next.CrateFrom, next.Dir); ok && push.CrateTo == next.CrateTo {
			c.plan = c.plan[1:]
			return selection(push), nil
		}
	}
	c.plan = nil
	if sol, ok, err := solver.Solve(state); err == nil && ok && len(sol.Pushes) > 0 {
		c.plan = sol.Pushes[1:]
		first := sol.Pushes[0]
		return selection(boxxle.LegalPush{
			Crate: first.CrateFrom, Dir: first.Dir, PlayerFrom: first.PlayerFrom, CrateTo: first.CrateTo,
		}), nil
	}
	d, err := policy.Choose(state)
	if err != nil {
		return decision.Selection{}, err
	}
	return selection(d.Candidate.Push), nil
}

// Run plays a Boxxle puzzle: it boots the cartridge to the puzzle screen, then
// repeatedly observes the board, chooses a legal push (deterministic policy by
// default, or a bounded model selector when Choose is set), and executes it
// through the deterministic controller until the puzzle is solved, a budget is
// exhausted, or the run is cancelled. The model only ever selects from the
// declared legal-push set; the controller owns all D-pad input.
func Run(profile game.CartridgeProfile, m Machine, opts RunOptions) Result {
	telemetry := &decision.Telemetry{}
	record := func(sel decision.Selection) {
		telemetry.Record(sel)
		if opts.OnDecision != nil {
			opts.OnDecision(sel)
		}
	}
	snapshot := func() *decision.Snapshot {
		s := telemetry.Snapshot()
		return &s
	}

	state, err := bootToPuzzle(profile, m)
	if err != nil {
		return Result{Reason: "error", State: state, Telemetry: snapshot(), Err: err}
	}

	choose := opts.Choose
	if choose == nil {
		choose = (&solverChooser{}).choose
	}
	want := max(opts.Levels, 1)
	startFrame := m.FrameCount()
	pushes, levels := 0, 0
	var lastSelection *decision.Selection
	result := func(reason string, state boxxle.State, err error) Result {
		return Result{Reason: reason, Pushes: pushes, Levels: levels, State: state, LastSelection: lastSelection, Telemetry: snapshot(), Err: err}
	}
	for {
		if cancelled(opts.Cancel) {
			return finish(m, result, "cancelled")
		}
		if opts.MaxFrames > 0 && int(m.FrameCount()-startFrame) >= opts.MaxFrames {
			return finish(m, result, "budget")
		}

		state, err := boxxle.DecodeState(m)
		if err != nil {
			return result("error", state, err)
		}
		if state.Solved {
			levels++
			// Advance even after the last level: leaving the solved screen
			// for a fresh board is the positive proof the puzzle completed.
			next, err := advance(m)
			if err != nil {
				return result("error", state, err)
			}
			if levels >= want {
				return result("done", next, nil)
			}
			continue
		}
		if opts.MaxPushes > 0 && pushes >= opts.MaxPushes {
			return result("budget", state, nil)
		}
		if state.Screen != boxxle.ScreenPuzzle {
			// Not on a puzzle screen (transition): settle and re-observe.
			m.StepFrame()
			continue
		}

		selection, err := choose(state)
		if err != nil {
			return result("error", state, err)
		}
		lastSelection = &selection
		record(selection)

		res, err := control.Push(m, decoder{}, selection.Plan.Push)
		if err != nil {
			// A failed push reports the board it failed on: After is only
			// set once the push itself has run.
			failed := res.After
			if failed.Screen == "" {
				failed = res.Before
			}
			return result("error", failed, err)
		}
		pushes++
	}
}

// Input script, measured on the real ROM: the cartridge idles on its title for
// a moment, Start opens the menu, Start on PLAY begins the intro, and A skips
// the intro into the first puzzle. After a solve, A alone walks the
// level-complete screens into the next puzzle.
const (
	bootIdleFrames  = 300 // let the title finish drawing before the first input
	pressFrames     = 8
	settleFrames    = 120 // after a press, how long to wait for the board
	stableFrames    = 20  // a board must decode identically this long
	advanceFrameCap = 1800
)

var bootScript = []emu.Button{emu.Start, emu.Start}

// ErrAdvanceTimeout reports that a solved puzzle never gave way to the next.
var ErrAdvanceTimeout = errors.New("boxxle session: advance to next puzzle timed out")

// bootToPuzzle drives the title and menu into the first puzzle and returns the
// stable board.
func bootToPuzzle(profile game.CartridgeProfile, m Machine) (boxxle.State, error) {
	// The title only accepts input once drawn. Boot has usually settled this
	// far already; an absolute frame target keeps the two from adding up.
	for m.FrameCount() < launchSettleFrames {
		m.StepFrame()
	}
	state, ok := enterPuzzle(m, bootScript, bootFrameBudget)
	if !ok {
		return state, fmt.Errorf("%w after %d frames (screen %s)", ErrBootTimeout, bootFrameBudget, state.Screen)
	}
	return state, nil
}

// advance leaves a solved puzzle for the next one and returns its board.
func advance(m Machine) (boxxle.State, error) {
	state, ok := enterPuzzle(m, nil, advanceFrameCap)
	if !ok {
		return state, fmt.Errorf("%w after %d frames (screen %s)", ErrAdvanceTimeout, advanceFrameCap, state.Screen)
	}
	return state, nil
}

// enterPuzzle presses the script, then A, until an unsolved board holds still.
// Progress is judged by decoded state, never by elapsed frames alone, and
// input is sent only while no such board is up, so a board already on screen
// is never disturbed.
func enterPuzzle(m Machine, script []emu.Button, budget int) (boxxle.State, bool) {
	start := m.FrameCount()
	over := func() bool { return int(m.FrameCount()-start) >= budget }

	// stable steps up to within frames and reports the unsolved board once it
	// has decoded identically for stableFrames.
	stable := func(within int) (boxxle.State, bool) {
		var last boxxle.State
		run := 0
		for i := 0; i < within && !over(); i++ {
			m.StepFrame()
			state, err := boxxle.DecodeState(m)
			if err != nil || state.Screen != boxxle.ScreenPuzzle || state.Player == nil {
				run = 0
				continue
			}
			if run > 0 && reflect.DeepEqual(state, last) {
				run++
			} else {
				run = 1
			}
			last = state
			if run >= stableFrames {
				return state, true
			}
		}
		return last, false
	}

	for n := 0; !over(); n++ {
		if state, ok := stable(2 * stableFrames); ok {
			return state, true
		}
		button := emu.A // past the script, A advances whatever is left
		if n < len(script) {
			button = script[n]
		}
		m.Press(button)
		for i := 0; i < pressFrames; i++ {
			m.StepFrame()
		}
		m.Release(button)
		if state, ok := stable(settleFrames); ok {
			return state, true
		}
	}
	state, _ := boxxle.DecodeState(m)
	return state, false
}

// finish re-observes the board and records a terminal result.
func finish(m Machine, result func(string, boxxle.State, error) Result, reason string) Result {
	state, err := boxxle.DecodeState(m)
	return result(reason, state, err)
}

func cancelled(cancel <-chan struct{}) bool {
	if cancel == nil {
		return false
	}
	select {
	case <-cancel:
		return true
	default:
		return false
	}
}
