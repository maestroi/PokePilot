package main

import (
	"context"
	"fmt"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/boxxle"
	boxxledecision "github.com/maestroi/pokepilot/boxxle/decision"
	boxxlsession "github.com/maestroi/pokepilot/boxxle/session"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// runLocalBoxxle is the local launch-only path for a Boxxle cartridge. It
// registers and launches Boxxle through the standard run path but does not play
// it, keeping the screen server up so a human can watch the title screen, then
// stops.
func runLocalBoxxle(m *emu.Emu, hold time.Duration, served string) {
	fmt.Println("Boxxle is registered and launched; autonomous puzzle play is a later slice.")
	fmt.Printf("still serving http://%s for %s, ctrl-c to quit\n", served, hold)
	m.Pace(60)
	for deadline := time.Now().Add(hold); time.Now().Before(deadline); {
		m.StepFrames(4)
	}
}

// runLocalBoxxlePlay is the local autonomous puzzle path for a Boxxle
// cartridge. It boots the cartridge to the puzzle screen, then runs the
// observe/choose/execute loop: a bounded selector (deterministic policy by
// default, or a model planner from the decision settings) picks a legal
// semantic push and the deterministic controller executes it. The model never
// emits raw D-pad input.
func runLocalBoxxlePlay(m *emu.Emu, profile game.CartridgeProfile, maxPushes, maxChoices int) {
	settings := agent.DecisionSettingsFromEnv()
	var (
		choose        func(boxxle.State) (boxxledecision.Selection, error)
		lastSelection *boxxledecision.Selection
	)
	if settings.Engine != nil {
		selector := boxxledecision.Selector{
			Engine: settings.Engine, MinConfidence: settings.MinConfidence,
			Shadow: settings.Shadow, MaxChoices: maxChoices,
		}
		fmt.Printf("boxxle typed pushes: backend=%s mode=%s max_choices=%d\n", settings.Backend, settings.Mode(), maxChoices)
		choose = func(state boxxle.State) (boxxledecision.Selection, error) {
			selection, err := selector.Choose(context.Background(), state)
			if err != nil {
				return boxxledecision.Selection{}, err
			}
			lastSelection = &selection
			return selection, nil
		}
	}
	result := boxxlsession.Run(profile, m, boxxlsession.RunOptions{
		MaxPushes: maxPushes,
		Choose:    choose,
		OnDecision: func(sel boxxledecision.Selection) {
			fmt.Printf("boxxle: push %s (confidence=%.2f fallback=%v shadow=%v)\n",
				boxxlePushLabel(sel.Plan.Push), sel.Response.Confidence, sel.Fallback, sel.Shadow)
			if lastSelection != nil && lastSelection.DecisionErr != nil {
				fmt.Printf("  typed push fallback: %v\n", lastSelection.DecisionErr)
			}
		},
	})
	fmt.Printf("boxxle stopped: %s after %d push(es)\n", result.Reason, result.Pushes)
	if result.Telemetry != nil {
		fmt.Printf("boxxle telemetry: backend=%s model=%s replans=%d fallbacks=%d invalid=%d latency=%s\n",
			result.Telemetry.Backend, result.Telemetry.Model, result.Telemetry.Replans,
			result.Telemetry.Fallbacks, result.Telemetry.Invalid, result.Telemetry.Latency)
	}
	if result.Err != nil {
		panic(fmt.Sprintf("boxxle run: %v", result.Err))
	}
}

// boxxlePushLabel renders a legal push as a short human-readable label.
func boxxlePushLabel(push boxxle.LegalPush) string {
	return fmt.Sprintf("crate (%d,%d) -> (%d,%d) %s", push.Crate.X, push.Crate.Y, push.CrateTo.X, push.CrateTo.Y, push.Dir)
}
