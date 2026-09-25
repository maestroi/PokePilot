package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

type healRuntimeState struct {
	Map          uint8
	X, Y         uint8
	Controllable bool
}

func healRuntimeStateWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder) (healRuntimeState, error) {
	if decoder == nil {
		return healRuntimeState{}, fmt.Errorf("skill: Heal: nil overworld decoder")
	}
	state := decoder.DecodeOverworld(reader)
	if state.NativeMapID > 0xff {
		return healRuntimeState{}, fmt.Errorf("skill: Heal: native map id %#04x exceeds current routing range", state.NativeMapID)
	}
	return healRuntimeState{
		Map:          uint8(state.NativeMapID),
		X:            state.X,
		Y:            state.Y,
		Controllable: state.Controllable,
	}, nil
}
