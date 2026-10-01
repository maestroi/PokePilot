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

// runFarmBoxxle is the farm launch-only path for a Boxxle cartridge. The
// cartridge is booted by prepareFarmAttempt; this slice registers and launches
// Boxxle but does not play it, so the run finishes as registered.
func runFarmBoxxle(m *emu.Emu, spec farm.Spec) (string, string) {
	return "registered", "Boxxle launched; autonomous puzzle play is a later slice"
}

// runFarmBoxxlePlay is the farm autonomous puzzle path for a Boxxle cartridge.
// It runs the observe/choose/execute loop: a bounded selector (deterministic
// policy by default, or a model planner from the decision spec) picks a legal
// semantic push and the deterministic controller executes it. The model never
// emits raw D-pad input.
func runFarmBoxxlePlay(
	m *emu.Emu,
	profile game.CartridgeProfile,
	spec farm.Spec,
	maxPushes, maxFrames int,
	cancel <-chan struct{},
	snap *heartbeatSnap,
) (string, string) {
	var choose func(boxxle.State) (boxxledecision.Selection, error)
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
		choose = func(state boxxle.State) (boxxledecision.Selection, error) {
			return selector.Choose(context.Background(), state)
		}
		m.TraceNote("boxxle", fmt.Sprintf("typed pushes backend=%s mode=%s min_confidence=%.2f max_choices=%d", settings.Backend, settings.Mode(), settings.MinConfidence, decisionSpec.MaxChoices))
	}

	result := boxxlsession.Run(profile, m, boxxlsession.RunOptions{
		MaxPushes: maxPushes,
		MaxFrames: maxFrames,
		Cancel:    cancel,
		Choose:    choose,
		OnDecision: func(sel boxxledecision.Selection) {
			if snap == nil {
				return
			}
			snap.storePlan("", fmt.Sprintf("push %s", boxxlePushLabel(sel.Plan.Push)))
		},
	})

	detail := fmt.Sprintf("%d push(es), %d puzzle(s) solved", result.Pushes, result.Levels)
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
