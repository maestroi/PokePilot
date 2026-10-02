package main

import (
	"context"
	"fmt"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/boxxle"
	boxxledecision "github.com/maestroi/pokepilot/boxxle/decision"
	boxxlsession "github.com/maestroi/pokepilot/boxxle/session"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/game"
)

// boxxleProgress is the live puzzle-count the heartbeat sample cannot see
// from RAM alone: how many pushes and solved boards this attempt has done.
type boxxleProgress struct {
	pushes int
	levels int
}

// runFarmBoxxle is the farm launch-only path for a Boxxle cartridge. The
// cartridge is booted by prepareFarmAttempt; this path registers and launches
// Boxxle but does not play it, so the run finishes as registered.
func runFarmBoxxle(m *emu.Emu, spec farm.Spec) (string, string) {
	return "registered", "Boxxle launched without autonomous play"
}

// runFarmBoxxlePlay is the farm autonomous puzzle path for a Boxxle cartridge.
// It runs the observe/choose/execute loop: a bounded selector (deterministic
// solver by default, or a model planner from the decision spec) picks a legal
// semantic push and the deterministic controller executes it. The model never
// emits raw D-pad input.
func runFarmBoxxlePlay(
	m *emu.Emu,
	profile game.CartridgeProfile,
	spec farm.Spec,
	maxPushes, maxFrames int,
	cancel <-chan struct{},
	snap *heartbeatSnap,
	progress *boxxleProgress,
) (string, string) {
	goal, err := boxxlsession.ParseGoal(spec.Goal.String())
	if err != nil {
		return "error", err.Error()
	}
	m.TraceNote("boxxle", fmt.Sprintf("goal=%s levels=%d", goal.String(), goal.WantedLevels()))

	var (
		choose        func(boxxle.State) (boxxledecision.Selection, error)
		lastSelection *boxxledecision.Selection
	)
	if decisionSpec := spec.DecisionEngine; decisionSpec != nil && decisionSpec.Enabled() {
		if !decisionSpec.Placements {
			return "error", "boxxle decision engine selected without placements enabled"
		}
		if decisionSpec.Battles || decisionSpec.Objectives || decisionSpec.Failures {
			return "error", "boxxle decision engine supports push decisions only"
		}
		settings, err := agent.DecisionSettingsFor(decisionSelectionFor(decisionSpec))
		if err != nil {
			return "error", fmt.Sprintf("boxxle decision engine: %v", err)
		}
		selector := boxxledecision.Selector{
			Engine: settings.Engine, MinConfidence: settings.MinConfidence,
			Shadow: settings.Shadow, MaxChoices: decisionSpec.MaxChoices,
		}
		solver := boxxlsession.NewSolverChooser()
		choose = func(state boxxle.State) (boxxledecision.Selection, error) {
			selection, err := selector.Choose(context.Background(), state)
			if err != nil {
				return boxxledecision.Selection{}, err
			}
			if selection.Fallback {
				solved, solverErr := solver.Choose(state)
				if solverErr == nil {
					selection.Plan = solved.Plan
				}
			}
			lastSelection = &selection
			return selection, nil
		}
		m.TraceNote("boxxle", fmt.Sprintf("typed pushes backend=%s mode=%s min_confidence=%.2f max_choices=%d", settings.Backend, settings.Mode(), settings.MinConfidence, decisionSpec.MaxChoices))
	}

	result := boxxlsession.Run(profile, m, boxxlsession.RunOptions{
		Goal:      goal,
		MaxPushes: maxPushes,
		MaxFrames: maxFrames,
		Cancel:    cancel,
		Choose:    choose,
		OnProgress: func(state boxxle.State, pushes, levels int) {
			if progress != nil {
				progress.pushes = pushes
				progress.levels = levels
			}
			if snap == nil {
				return
			}
			snap.storeStatus(farm.Heartbeat{
				RunID:     spec.RunID,
				Frame:     m.FrameCount(),
				GameState: boxxleStateEnvelope(state, pushes, levels),
			})
		},
		OnDecision: func(sel boxxledecision.Selection) {
			if snap == nil {
				return
			}
			snap.storeGameDecision(boxxleDecisionEnvelope(sel, lastSelection))
			snap.storePlan("", fmt.Sprintf("push %s", boxxlePushLabel(sel.Plan.Push)))
		},
	})

	detail := fmt.Sprintf("%d push(es), %d puzzle(s) solved", result.Pushes, result.Levels)
	if result.UsedFallback {
		detail += ", solver fallback used"
	} else {
		detail += ", no solver fallback"
	}
	if result.Telemetry != nil && result.Telemetry.Backend != "" {
		detail += fmt.Sprintf(", backend=%s model=%s replans=%d fallbacks=%d invalid=%d",
			result.Telemetry.Backend, result.Telemetry.Model, result.Telemetry.Replans,
			result.Telemetry.Fallbacks, result.Telemetry.Invalid)
	}
	if result.Err != nil {
		return "error", detail + ": " + result.Err.Error()
	}
	switch result.Reason {
	case "done":
		return "done", detail
	case "cancelled":
		return "cancelled", detail
	case "budget":
		return "budget", detail
	default:
		return "error", fmt.Sprintf("unknown Boxxle stop %q · %s", result.Reason, detail)
	}
}

func sampleBoxxleHeartbeat(
	m *emu.Emu,
	profile game.CartridgeProfile,
	runID string,
	snap *heartbeatSnap,
	addrs []string,
	progress *boxxleProgress,
) {
	state, err := boxxle.Observe(profile, m)
	if err != nil {
		return
	}
	pushes, levels := 0, 0
	if progress != nil {
		pushes, levels = progress.pushes, progress.levels
	}
	hb := farm.Heartbeat{
		RunID:       runID,
		Frame:       m.FrameCount(),
		WorkerAddrs: addrs,
		GameState:   boxxleStateEnvelope(state, pushes, levels),
	}
	if tail := m.TraceTail(1); len(tail) > 0 {
		hb.Trace = tail[len(tail)-1]
	}
	snap.storeStatus(hb)
}

func boxxleStateEnvelope(state boxxle.State, pushes, levels int) map[string]any {
	onGoal := boxxle.CratesOnGoal(state)
	out := map[string]any{
		"kind":           "boxxle",
		"screen":         string(state.Screen),
		"width":          state.Width,
		"height":         state.Height,
		"board":          boxxle.RenderBoard(state),
		"solved":         state.Solved,
		"pushes":         pushes,
		"levels":         levels,
		"crates":         len(state.Crates),
		"goals":          len(state.Goals),
		"crates_on_goal": onGoal,
	}
	if state.Player != nil {
		out["player"] = map[string]any{"x": state.Player.X, "y": state.Player.Y}
	}
	return out
}

func boxxleDecisionEnvelope(sel boxxledecision.Selection, selections ...*boxxledecision.Selection) map[string]any {
	push := sel.Plan.Push
	out := map[string]any{
		"kind":        "boxxle-push",
		"crate":       map[string]any{"x": push.Crate.X, "y": push.Crate.Y},
		"crate_to":    map[string]any{"x": push.CrateTo.X, "y": push.CrateTo.Y},
		"player_from": map[string]any{"x": push.PlayerFrom.X, "y": push.PlayerFrom.Y},
		"dir":         push.Dir.String(),
		"fallback":    sel.Fallback,
	}
	chosen := sel
	if len(selections) > 0 && selections[0] != nil {
		chosen = *selections[0]
	}
	mode := "active"
	if chosen.Shadow {
		mode = "shadow"
	}
	out["decision_mode"] = mode
	out["decision_backend"] = chosen.Response.Backend
	out["decision_model"] = chosen.Response.Model
	out["decision_choice"] = chosen.Response.Choice
	out["decision_confidence"] = chosen.Response.Confidence
	out["decision_fallback"] = chosen.Fallback
	if chosen.DecisionErr != nil {
		out["decision_error"] = chosen.DecisionErr.Error()
	}
	if chosen.Agreed != nil {
		out["decision_agreed"] = *chosen.Agreed
	}
	det := chosen.Deterministic.Push
	out["policy_crate"] = map[string]any{"x": det.Crate.X, "y": det.Crate.Y}
	out["policy_dir"] = det.Dir.String()
	return out
}
