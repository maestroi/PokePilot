package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

func navigationStateFromOverworld(state game.OverworldState) (navigationState, error) {
	if state.NativeMapID > 0xff {
		return navigationState{}, fmt.Errorf("skill: navigation: native map id %#04x exceeds current routing range", state.NativeMapID)
	}
	return navigationState{Map: uint8(state.NativeMapID), X: state.X, Y: state.Y}, nil
}

// navigationStateWithDecoder projects the portable overworld state into the
// current Gen-I-sized routing key. Destination/world graph map ids are still
// uint8 in this migration stage, so wider native map ids fail explicitly
// instead of being truncated.
func navigationStateWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder) (navigationState, error) {
	if decoder == nil {
		return navigationState{}, fmt.Errorf("skill: navigation: nil overworld decoder")
	}
	return navigationStateFromOverworld(decoder.DecodeOverworld(reader))
}

func abortIfBattleWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder) error {
	if decoder == nil {
		return fmt.Errorf("skill: navigation: nil overworld decoder")
	}
	overworld := decoder.DecodeOverworld(reader)
	state, err := navigationStateFromOverworld(overworld)
	if err != nil {
		return err
	}
	if overworld.InBattle {
		return fmt.Errorf("skill: GoTo: battle on map %02x at (%d,%d): %w",
			state.Map, state.X, state.Y, ErrBattle)
	}
	return nil
}
