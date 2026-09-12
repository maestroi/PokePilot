package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// DismissableObjectiveMenu reports a menu that an objective lifecycle may
// safely back out of with B. Gameplay questions are intentionally excluded:
// a two-option prompt belongs to the skill that encountered it, while ordinary
// cursor/list/party/PC surfaces are safe cancellation boundaries.
//
// The typed interaction decoder also distinguishes short scrolling item lists
// from two-option prompts even when both have wMaxMenuItem==1, so this layer no
// longer needs to inspect a raw list-menu ID itself.
func DismissableObjectiveMenu(mem *state.Mem) bool {
	switch state.DecodeInteraction(mem).Kind {
	case state.InteractionMenu, state.InteractionListMenu, state.InteractionElevatorMenu,
		state.InteractionItemMenu, state.InteractionPartyMenu, state.InteractionPCMenu,
		state.InteractionPCPokemonList:
		return true
	default:
		return false
	}
}

// CloseOpenMenuToOverworld backs out of leftover menu layers to a controllable
// overworld boundary. Every B press is preceded by a fresh typed interaction
// decode and followed by a verified transition. This is intentionally not the
// older "press B until controllable" loop: a B may expose a choice, battle, or
// other owner-controlled surface, and another blind B there could make a real
// gameplay decision.
func CloseOpenMenuToOverworld(m *emu.Emu) error {
	const maxLayers = 8
	for layer := 0; layer < maxLayers; layer++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		interaction := state.DecodeInteraction(&mem)
		if state.Controllable(&mem) && interaction.Kind == state.InteractionNone {
			return nil
		}
		if state.DecodeBattle(&mem) != nil {
			return fmt.Errorf("skill: CloseOpenMenuToOverworld: battle owns the screen")
		}
		switch interaction.Kind {
		case state.InteractionMenu, state.InteractionListMenu, state.InteractionElevatorMenu,
			state.InteractionItemMenu, state.InteractionPartyMenu, state.InteractionPCMenu,
			state.InteractionPCPokemonList:
			if err := CancelInteraction(m); err != nil {
				return fmt.Errorf("skill: CloseOpenMenuToOverworld: cancel layer %d (%s): %w", layer, interaction.Kind, err)
			}
		case state.InteractionTwoOption:
			return fmt.Errorf("skill: CloseOpenMenuToOverworld: choice remains open: %q", interaction.Text)
		case state.InteractionDialogue:
			return fmt.Errorf("skill: CloseOpenMenuToOverworld: dialogue remains open: %q", interaction.Text)
		case state.InteractionNone:
			// Menu teardown can clear the tilemap/font a few frames before
			// overworld control is restored. Wait without input for either
			// controllability or a newly decoded owner instead of treating
			// that legitimate in-flight state as a dirty boundary.
			_, _ = m.StepUntil(interactionTransitionFrames, func(e *emu.Emu) bool {
				var next state.Mem
				state.Snapshot(e, &next)
				return state.Controllable(&next) || state.DecodeInteraction(&next).Kind != state.InteractionNone || state.DecodeBattle(&next) != nil
			})
			continue
		default:
			return fmt.Errorf("skill: CloseOpenMenuToOverworld: unsupported interaction %q", interaction.Kind)
		}
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: CloseOpenMenuToOverworld: menu cleanup exceeded %d layers; final=%+v", maxLayers, state.DecodeInteraction(&mem))
}
