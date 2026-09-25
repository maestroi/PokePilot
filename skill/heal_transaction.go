package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

const (
	healMenuBudget           = 3000
	healRunBudget            = 30000
	healBoundaryStableFrames = talkSettle
)

type pokemonCenterRuntime interface {
	game.CenterDecoder
	game.OverworldDecoder
	game.MenuDecoder
}

type pokemonCenterMachine interface {
	menuMachine
}

func pokemonCenterRuntimeFor(m *emu.Emu) (pokemonCenterRuntime, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: Heal: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: Heal: detect profile: %w", err)
	}
	runtime, ok := profile.(pokemonCenterRuntime)
	if !ok {
		return nil, fmt.Errorf("skill: Heal: profile %s@%s does not expose Pokemon Center transaction semantics", profile.ID(), profile.Revision())
	}
	return runtime, nil
}

func healAtNurse(m pokemonCenterMachine, runtime pokemonCenterRuntime) error {
	if runtime == nil {
		return fmt.Errorf("skill: Heal: nil Pokemon Center runtime")
	}
	if err := openPokemonCenterPrompt(m, runtime); err != nil {
		return err
	}
	if err := selectTwoOptionWithDecoder(m, runtime, 0); err != nil {
		return fmt.Errorf("skill: Heal: select YES: %w", err)
	}
	if err := waitForPokemonCenterRecovery(m, runtime); err != nil {
		return err
	}
	return settlePokemonCenterBoundary(m, runtime, healRunBudget)
}

func openPokemonCenterPrompt(m pokemonCenterMachine, runtime pokemonCenterRuntime) error {
	m.Tap(emu.A, 3, 7)
	sawDialogue := false
	for i := 0; i < healMenuBudget; i++ {
		state := runtime.DecodeCenter(m)
		if state.PromptOpen {
			return nil
		}
		if state.TextOpen {
			sawDialogue = true
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
			continue
		}
		if state.MenuOpen {
			return fmt.Errorf("skill: Heal: unexpected menu before nurse prompt")
		}
		m.StepFrame()
	}
	if !sawDialogue {
		return fmt.Errorf("skill: Heal: %w: nurse dialogue did not open", ErrNoDialogue)
	}
	return fmt.Errorf("skill: Heal: yes/no prompt did not appear within %d iterations", healMenuBudget)
}

func waitForPokemonCenterRecovery(m pokemonCenterMachine, runtime pokemonCenterRuntime) error {
	for i := 0; i < healRunBudget; i++ {
		state := runtime.DecodeCenter(m)
		if state.Recovered {
			return nil
		}
		if state.PromptOpen {
			return fmt.Errorf("skill: Heal: unexpected choice while healing")
		}
		if state.TextOpen {
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
			continue
		}
		if state.MenuOpen {
			return fmt.Errorf("skill: Heal: unexpected menu while healing")
		}
		m.StepFrame()
	}
	return fmt.Errorf("skill: Heal: party was not fully recovered within %d frames", healRunBudget)
}

// settlePokemonCenterBoundary drains late ordinary nurse text but never
// answers a choice. A sustained semantic controllable state is required so a
// one-frame idle gap cannot be mistaken for transaction completion.
func settlePokemonCenterBoundary(m pokemonCenterMachine, runtime pokemonCenterRuntime, budget int) error {
	stable := 0
	for i := 0; i < budget; i++ {
		center := runtime.DecodeCenter(m)
		if center.PromptOpen {
			return fmt.Errorf("skill: Heal: unexpected choice while settling completed heal")
		}
		if center.MenuOpen && !center.TextOpen {
			return fmt.Errorf("skill: Heal: unexpected menu while settling completed heal")
		}
		if center.TextOpen {
			stable = 0
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
			continue
		}
		if runtime.DecodeOverworld(m).Controllable {
			stable++
			if stable >= healBoundaryStableFrames {
				return nil
			}
		} else {
			stable = 0
		}
		m.StepFrame()
	}
	world := runtime.DecodeOverworld(m)
	return fmt.Errorf("skill: Heal: completed heal did not reach a stable controllable boundary within %d frames: map=%#04x at (%d,%d)",
		budget, world.NativeMapID, world.X, world.Y)
}
