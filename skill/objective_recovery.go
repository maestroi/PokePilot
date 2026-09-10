package skill

import (
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

// CloseOpenMenuToOverworld backs out of a menu that a finished objective
// accidentally left open. The caller must first establish that the screen is
// dismissable (see DismissableObjectiveMenu), not a battle or an unanswered
// gameplay choice. B is a safe "back" operation for the former and could
// answer/alter the latter.
//
// Objective execution owns this cleanup as part of finishing its transaction:
// one failed bag/TM/party interaction must not poison the next objective with
// a non-controllable start, but this layer never makes a gameplay decision to
// achieve that invariant.
func CloseOpenMenuToOverworld(m *emu.Emu) error {
	return closeToOverworld(m)
}
