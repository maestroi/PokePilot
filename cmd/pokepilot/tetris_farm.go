package main

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/tetris"
	tetrispolicy "github.com/maestroi/pokepilot/tetris/policy"
	tetrissession "github.com/maestroi/pokepilot/tetris/session"
)

func runFarmTetris(
	m *emu.Emu,
	profile game.CartridgeProfile,
	spec farm.Spec,
	maxPieces, maxFrames int,
	cancel <-chan struct{},
	snap *heartbeatSnap,
) (string, string) {
	goal, err := tetrissession.ParseGoal(spec.Goal.String())
	if err != nil {
		return "error", err.Error()
	}
	if _, err := tetrissession.BootToPlaying(profile, m, goal.Mode()); err != nil {
		return "error", err.Error()
	}

	m.TraceNote("tetris", fmt.Sprintf("goal=%s objective=%s mode=%s", tetrisGoalLabel(goal), goal.Objective, goal.Mode()))
	result := tetrissession.Run(profile, m, tetrissession.RunOptions{
		Goal:      goal,
		MaxPieces: maxPieces,
		MaxFrames: maxFrames,
		Cancel:    cancel,
		OnDecision: func(decision tetrispolicy.Decision) {
			if snap == nil {
				return
			}
			snap.storeGameDecision(tetrisDecisionEnvelope(decision))
			snap.storePlan("", fmt.Sprintf(
				"%s %s → rotation %d, column %d",
				decision.Objective,
				decision.Piece,
				decision.Candidate.Placement.Rotation,
				decision.Candidate.Placement.Column,
			))
		},
	})

	detail := fmt.Sprintf(
		"%d piece(s), score %d, lines %d, level %d",
		result.Pieces,
		result.State.Score,
		result.State.LinesCleared,
		result.State.Level,
	)
	if result.Err != nil {
		return "error", detail + ": " + result.Err.Error()
	}
	switch result.Reason {
	case "done":
		return "done", detail
	case "game-over":
		if goal.Kind == tetrissession.GoalAuto || goal.Kind == tetrissession.GoalSurvival {
			return "done", "game over · " + detail
		}
		return "failed", "game over before " + tetrisGoalLabel(goal) + " · " + detail
	case "cancelled":
		return "cancelled", detail
	case "budget":
		return "budget", detail
	default:
		return "error", fmt.Sprintf("unknown Tetris stop %q · %s", result.Reason, detail)
	}
}

func sampleTetrisHeartbeat(
	m *emu.Emu,
	profile game.CartridgeProfile,
	runID string,
	snap *heartbeatSnap,
	addrs []string,
) {
	state, err := tetris.Observe(profile, m)
	if err != nil {
		return
	}
	hb := farm.Heartbeat{
		RunID:       runID,
		Frame:       m.FrameCount(),
		WorkerAddrs: addrs,
		GameState:   tetrisStateEnvelope(state),
	}
	if tail := m.TraceTail(1); len(tail) > 0 {
		hb.Trace = tail[len(tail)-1]
	}
	snap.storeStatus(hb)
}

func tetrisStateEnvelope(state tetris.State) map[string]any {
	board := make([]string, tetris.BoardHeight)
	for y := 0; y < tetris.BoardHeight; y++ {
		var row strings.Builder
		row.Grow(tetris.BoardWidth)
		for x := 0; x < tetris.BoardWidth; x++ {
			if state.Board[y][x] {
				row.WriteByte('#')
			} else {
				row.WriteByte('.')
			}
		}
		board[y] = row.String()
	}
	out := map[string]any{
		"kind":                  "tetris",
		"mode":                  string(state.Mode),
		"screen":                string(state.Screen),
		"board":                 board,
		"level":                 state.Level,
		"score":                 state.Score,
		"score_valid":           state.ScoreValid,
		"lines_cleared":         state.LinesCleared,
		"lines_remaining":       state.LinesRemaining,
		"line_goal":             state.LineGoal,
		"paused":                state.Paused,
		"locking":               state.Locking,
		"clearing":              state.Clearing,
		"game_over":             state.GameOver,
		"complete":              state.Complete,
		"ready_for_piece_input": state.ReadyForPieceInput,
	}
	if state.Active != nil {
		out["active"] = map[string]any{
			"piece":    string(state.Active.Piece),
			"rotation": state.Active.Rotation,
			"x":        state.Active.X,
			"y":        state.Active.Y,
		}
	}
	if state.Next != nil {
		out["next"] = map[string]any{
			"piece":    string(state.Next.Piece),
			"rotation": state.Next.Rotation,
		}
	}
	return out
}

func tetrisDecisionEnvelope(decision tetrispolicy.Decision) map[string]any {
	c := decision.Candidate
	return map[string]any{
		"kind":                "tetris-placement",
		"objective":           string(decision.Objective),
		"piece":               string(decision.Piece),
		"considered":          decision.Considered,
		"rotation":            c.Placement.Rotation,
		"column":              c.Placement.Column,
		"landing_y":           c.LandingY,
		"lines_cleared":       c.LinesCleared,
		"expected_line_score": c.ExpectedLineScore,
		"immediate_score":     c.ImmediateScore,
		"lookahead_score":     c.LookaheadScore,
		"total_score":         c.TotalScore,
		"input_steps":         c.InputSteps,
		"metrics": map[string]any{
			"aggregate_height": c.Metrics.AggregateHeight,
			"max_height":       c.Metrics.MaxHeight,
			"holes":            c.Metrics.Holes,
			"bumpiness":        c.Metrics.Bumpiness,
			"wells":            c.Metrics.Wells,
		},
	}
}

func tetrisGoalLabel(goal tetrissession.Goal) string {
	switch goal.Kind {
	case tetrissession.GoalScore, tetrissession.GoalLines:
		return fmt.Sprintf("%s:%d", goal.Kind, goal.Target)
	default:
		return string(goal.Kind)
	}
}
