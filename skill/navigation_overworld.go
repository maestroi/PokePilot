package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

// navigationStateWithDecoder projects the portable overworld state into the
// current Gen-I-sized routing key. Destination/world graph map ids are still
// uint8 in this migration stage, so wider native map ids fail explicitly
// instead of being truncated.
func navigationStateWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder) (navigationState, error) {
	if decoder == nil {
		return navigationState{}, fmt.Errorf("skill: navigation: nil overworld decoder")
	}
	state := decoder.DecodeOverworld(reader)
	if state.NativeMapID > 0xff {
		return navigationState{}, fmt.Errorf("skill: navigation: native map id %#04x exceeds current routing range", state.NativeMapID)
	}
	return navigationState{Map: uint8(state.NativeMapID), X: state.X, Y: state.Y}, nil
}

func abortIfBattleWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder) error {
	state, err := navigationStateWithDecoder(reader, decoder)
	if err != nil {
		return err
	}
	if decoder.DecodeOverworld(reader).InBattle {
		return fmt.Errorf("skill: GoTo: battle on map %02x at (%d,%d): %w",
			state.Map, state.X, state.Y, ErrBattle)
	}
	return nil
}
