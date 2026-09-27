// Package session owns long-running Tetris game/session mechanics above the
// semantic policy/controller layers and below farm/operator integration.
package session

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/tetris"
	"github.com/maestroi/pokepilot/tetris/control"
	"github.com/maestroi/pokepilot/tetris/policy"
)

const (
	bootFrameBudget = 3600
)

var ErrBootTimeout = errors.New("tetris session: boot timed out")

type Machine interface {
	control.Machine
	FrameCount() uint64
}

type GoalKind string

const (
	GoalAuto     GoalKind = "auto"
	GoalSurvival GoalKind = "survival"
	GoalLines    GoalKind = "lines"
	GoalScore    GoalKind = "score"
	GoalComplete GoalKind = "complete"
)

type Goal struct {
	Kind      GoalKind         `json:"kind"`
	Target    int              `json:"target,omitempty"`
	Objective policy.Objective `json:"objective"`
}

func ParseGoal(raw string) (Goal, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "", "auto":
		return Goal{Kind: GoalAuto, Objective: policy.ObjectiveAuto}, nil
	case "survival":
		return Goal{Kind: GoalSurvival, Objective: policy.ObjectiveSurvival}, nil
	case "complete":
		return Goal{Kind: GoalComplete, Objective: policy.ObjectiveAuto}, nil
	}
	for _, spec := range []struct {
		prefix    string
		kind      GoalKind
		objective policy.Objective
	}{
		{"lines:", GoalLines, policy.ObjectiveLines},
		{"score:", GoalScore, policy.ObjectiveScore},
	} {
		if !strings.HasPrefix(raw, spec.prefix) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(raw, spec.prefix)))
		if err != nil || n <= 0 {
			return Goal{}, fmt.Errorf("tetris session: %s target must be a positive integer", spec.kind)
		}
		return Goal{Kind: spec.kind, Target: n, Objective: spec.objective}, nil
	}
	return Goal{}, fmt.Errorf("tetris session: unknown goal %q; want auto, survival, complete, lines:N, or score:N", raw)
}

func (g Goal) Mode() tetris.Mode {
	if g.Kind == GoalLines || g.Kind == GoalComplete {
		return tetris.ModeB
	}
	return tetris.ModeA
}

func (g Goal) Satisfied(state tetris.State) bool {
	switch g.Kind {
	case GoalLines:
		return state.LinesCleared >= g.Target || (state.Mode == tetris.ModeB && state.Complete)
	case GoalScore:
		return state.ScoreValid && state.Score >= g.Target
	case GoalComplete:
		return state.Complete
	default:
		return false
	}
}

// BootToTitle advances a freshly loaded cartridge until the one-player title
// menu is available. It sends no input, making the resulting state safe to
// cache as a per-cartridge farm boot state before a run chooses its mode.
func BootToTitle(profile game.CartridgeProfile, m Machine) (tetris.State, error) {
	start := m.FrameCount()
	for m.FrameCount()-start < bootFrameBudget {
		state, err := tetris.Observe(profile, m)
		if err == nil && state.Screen == tetris.ScreenTitle {
			return state, nil
		}
		m.StepFrame()
	}
	state, _ := tetris.Observe(profile, m)
	return state, fmt.Errorf("%w after %d frames (screen %s)", ErrBootTimeout, bootFrameBudget, state.Screen)
}

// BootToPlaying navigates the native one-player menus to a level-0 Type A or
// Type B game. Music/start-height settings remain their cartridge defaults.
func BootToPlaying(profile game.CartridgeProfile, m Machine, mode tetris.Mode) (tetris.State, error) {
	if mode != tetris.ModeA && mode != tetris.ModeB {
		return tetris.State{}, fmt.Errorf("tetris session: unsupported one-player mode %q", mode)
	}
	start := m.FrameCount()
	for m.FrameCount()-start < bootFrameBudget {
		state, err := tetris.Observe(profile, m)
		if err != nil {
			m.StepFrame()
			continue
		}
		if state.Screen == tetris.ScreenPlaying && state.ReadyForPieceInput {
			return state, nil
		}
		if state.GameOver || state.Complete {
			return state, fmt.Errorf("tetris session: terminal state while booting (%s)", state.Screen)
		}

		switch state.Screen {
		case tetris.ScreenTitle:
			pulse(m, emu.Start)
		case tetris.ScreenGameType:
			switch {
			case state.Mode == tetris.ModeUnknown:
				m.StepFrame()
			case mode == tetris.ModeA && state.Mode != tetris.ModeA:
				pulse(m, emu.Left)
			case mode == tetris.ModeB && state.Mode != tetris.ModeB:
				pulse(m, emu.Right)
			default:
				pulse(m, emu.Start)
			}
		case tetris.ScreenMusic:
			pulse(m, emu.Start)
		case tetris.ScreenLevel, tetris.ScreenHeight:
			pulse(m, emu.Start)
		default:
			m.StepFrame()
		}
	}
	state, _ := tetris.Observe(profile, m)
	return state, fmt.Errorf("%w after %d frames (screen %s mode %s)", ErrBootTimeout, bootFrameBudget, state.Screen, state.Mode)
}

type RunOptions struct {
	Goal       Goal
	MaxPieces  int
	MaxFrames  int
	Cancel     <-chan struct{}
	// Choose may replace the deterministic policy selector with another bounded
	// selector. The selector still returns a policy.Decision, so controller
	// execution remains unchanged. Nil preserves historical deterministic play.
	Choose     func(tetris.State, policy.Objective) (policy.Decision, error)
	OnDecision func(policy.Decision)
}

type Result struct {
	Reason       string           `json:"reason"`
	Pieces       int              `json:"pieces"`
	State        tetris.State     `json:"state"`
	LastDecision *policy.Decision `json:"last_decision,omitempty"`
	Err          error            `json:"-"`
}

func Run(profile game.CartridgeProfile, m Machine, opts RunOptions) Result {
	startFrame := m.FrameCount()
	pieces := 0
	var lastDecision *policy.Decision
	for {
		if cancelled(opts.Cancel) {
			return resultFromState(profile, m, "cancelled", nil, pieces, lastDecision)
		}
		if opts.MaxFrames > 0 && int(m.FrameCount()-startFrame) >= opts.MaxFrames {
			return resultFromState(profile, m, "budget", nil, pieces, lastDecision)
		}

		state, err := tetris.Observe(profile, m)
		if err != nil {
			return Result{Reason: "error", Pieces: pieces, LastDecision: lastDecision, Err: err}
		}
		if opts.Goal.Satisfied(state) || state.Complete {
			return Result{Reason: "done", Pieces: pieces, State: state, LastDecision: lastDecision}
		}
		if state.GameOver {
			return Result{Reason: "game-over", Pieces: pieces, State: state, LastDecision: lastDecision}
		}
		if !state.ReadyForPieceInput {
			m.StepFrame()
			continue
		}

		choose := opts.Choose
		if choose == nil {
			choose = policy.Choose
		}
		decision, err := choose(state, opts.Goal.Objective)
		if err != nil {
			return Result{Reason: "error", Pieces: pieces, State: state, LastDecision: lastDecision, Err: err}
		}
		lastDecision = &decision
		if opts.OnDecision != nil {
			opts.OnDecision(decision)
		}
		placement, err := control.Place(profile, m, decision.Candidate.Placement)
		if err != nil {
			return Result{Reason: "error", Pieces: pieces, State: placement.After, LastDecision: lastDecision, Err: err}
		}
		pieces++

		if opts.Goal.Satisfied(placement.After) || placement.After.Complete {
			return Result{Reason: "done", Pieces: pieces, State: placement.After, LastDecision: lastDecision}
		}
		if placement.After.GameOver {
			return Result{Reason: "game-over", Pieces: pieces, State: placement.After, LastDecision: lastDecision}
		}
		if opts.MaxPieces > 0 && pieces >= opts.MaxPieces {
			return Result{Reason: "budget", Pieces: pieces, State: placement.After, LastDecision: lastDecision}
		}
	}
}

func resultFromState(profile game.CartridgeProfile, m Machine, reason string, err error, pieces int, lastDecision *policy.Decision) Result {
	state, observeErr := tetris.Observe(profile, m)
	if err == nil {
		err = observeErr
	}
	return Result{Reason: reason, Pieces: pieces, State: state, LastDecision: lastDecision, Err: err}
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

func pulse(m Machine, button emu.Button) {
	m.Press(button)
	m.StepFrame()
	m.Release(button)
	m.StepFrame()
}
