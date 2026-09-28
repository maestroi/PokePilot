package session

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/control"
	"github.com/maestroi/pokepilot/boxxle/decision"
	"github.com/maestroi/pokepilot/boxxle/policy"
	"github.com/maestroi/pokepilot/game"
)

// ErrBootTimeout reports that the cartridge did not reach the puzzle screen
// within the boot budget.
var ErrBootTimeout = errors.New("boxxle session: boot timed out")

// RunOptions configures an autonomous Boxxle puzzle run.
type RunOptions struct {
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
	Reason        string              `json:"reason"`
	Pushes        int                 `json:"pushes"`
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

// deterministicChoose wraps the deterministic policy in a decision.Selection so
// the run loop has a single selector surface. The policy is the fallback plan
// and, absent a model, the executed plan.
func deterministicChoose(state boxxle.State) (decision.Selection, error) {
	d, err := policy.Choose(state)
	if err != nil {
		return decision.Selection{}, err
	}
	plan := decision.Plan{Push: d.Candidate.Push}
	return decision.Selection{Plan: plan, Deterministic: plan}, nil
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
	if state.Solved {
		return Result{Reason: "done", State: state, Telemetry: snapshot()}
	}

	startFrame := m.FrameCount()
	pushes := 0
	var lastSelection *decision.Selection
	for {
		if cancelled(opts.Cancel) {
			return finish(profile, m, "cancelled", nil, pushes, lastSelection, snapshot())
		}
		if opts.MaxFrames > 0 && int(m.FrameCount()-startFrame) >= opts.MaxFrames {
			return finish(profile, m, "budget", nil, pushes, lastSelection, snapshot())
		}

		state, err := boxxle.DecodeState(m)
		if err != nil {
			return Result{Reason: "error", Pushes: pushes, LastSelection: lastSelection, Telemetry: snapshot(), Err: err}
		}
		if state.Solved {
			return Result{Reason: "done", Pushes: pushes, State: state, LastSelection: lastSelection, Telemetry: snapshot()}
		}
		if state.Screen != boxxle.ScreenPuzzle && state.Screen != boxxle.ScreenSolved {
			// Not on a puzzle screen (title, menu, transition): settle and re-observe.
			m.StepFrame()
			continue
		}

		choose := opts.Choose
		if choose == nil {
			choose = deterministicChoose
		}
		selection, err := choose(state)
		if err != nil {
			return Result{Reason: "error", Pushes: pushes, State: state, LastSelection: lastSelection, Telemetry: snapshot(), Err: err}
		}
		lastSelection = &selection
		record(selection)

		res, err := control.Push(m, decoder{}, selection.Plan.Push)
		if err != nil {
			return Result{Reason: "error", Pushes: pushes, State: res.After, LastSelection: lastSelection, Telemetry: snapshot(), Err: err}
		}
		pushes++

		if res.After.Solved {
			return Result{Reason: "done", Pushes: pushes, State: res.After, LastSelection: lastSelection, Telemetry: snapshot()}
		}
		if opts.MaxPushes > 0 && pushes >= opts.MaxPushes {
			return Result{Reason: "budget", Pushes: pushes, State: res.After, LastSelection: lastSelection, Telemetry: snapshot()}
		}
	}
}

// bootToPuzzle advances the cartridge until the board is a usable puzzle,
// decoding state each frame so the settle terminates on the puzzle screen.
func bootToPuzzle(profile game.CartridgeProfile, m Machine) (boxxle.State, error) {
	start := m.FrameCount()
	for m.FrameCount()-start < bootFrameBudget {
		state, err := boxxle.DecodeState(m)
		if err == nil && state.Screen == boxxle.ScreenPuzzle {
			return state, nil
		}
		m.StepFrame()
	}
	state, _ := boxxle.DecodeState(m)
	return state, fmt.Errorf("%w after %d frames (screen %s)", ErrBootTimeout, bootFrameBudget, state.Screen)
}

// finish re-observes the board and records a terminal result.
func finish(profile game.CartridgeProfile, m Machine, reason string, err error, pushes int, lastSelection *decision.Selection, telemetry *decision.Snapshot) Result {
	state, observeErr := boxxle.DecodeState(m)
	if err == nil {
		err = observeErr
	}
	return Result{Reason: reason, Pushes: pushes, State: state, LastSelection: lastSelection, Telemetry: telemetry, Err: err}
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
