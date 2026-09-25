package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

// DecodeListMenu keeps Red/Blue list-menu identity, cursor bytes and scrolling
// semantics behind the profile boundary.
func (*Profile) DecodeListMenu(reader game.MemoryReader) game.ListMenuState {
	if reader == nil {
		return game.ListMenuState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	interaction := state.DecodeInteraction(&mem)

	kind := game.ListMenuKind("")
	switch interaction.Kind {
	case state.InteractionItemMenu:
		kind = game.ListMenuItems
	case state.InteractionElevatorMenu:
		kind = game.ListMenuElevator
	case state.InteractionPCPokemonList:
		kind = game.ListMenuPCPokemon
	case state.InteractionListMenu:
		kind = game.ListMenuGeneric
	default:
		return game.ListMenuState{}
	}

	return game.ListMenuState{
		Visible:  true,
		Kind:     kind,
		Position: interaction.Current,
	}
}
