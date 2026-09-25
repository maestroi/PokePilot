package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

type fieldPathRuntimeState struct {
	Map  uint8
	X, Y uint8
}

func fieldPathRuntimeStateWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder) (fieldPathRuntimeState, error) {
	if decoder == nil {
		return fieldPathRuntimeState{}, fmt.Errorf("skill: field path: nil overworld decoder")
	}
	state := decoder.DecodeOverworld(reader)
	if state.NativeMapID > 0xff {
		return fieldPathRuntimeState{}, fmt.Errorf("skill: field path: native map id %#04x exceeds current routing range", state.NativeMapID)
	}
	return fieldPathRuntimeState{Map: uint8(state.NativeMapID), X: state.X, Y: state.Y}, nil
}

func fieldPathActionTargetWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder, step fieldPathStep) (fieldPathRuntimeState, int, int, error) {
	live, err := fieldPathRuntimeStateWithDecoder(reader, decoder)
	if err != nil {
		return fieldPathRuntimeState{}, 0, 0, err
	}
	if absInt(step.Move.DX)+absInt(step.Move.DY) != 1 {
		return live, 0, 0, fmt.Errorf("skill: field path action %d has non-adjacent step %s", step.Action, step.Move)
	}
	return live, int(live.X) + step.Move.DX, int(live.Y) + step.Move.DY, nil
}
