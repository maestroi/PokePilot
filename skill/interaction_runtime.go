package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

type interactionRuntimeState struct {
	Map      uint8
	X, Y     uint8
	InBattle bool
}

func interactionRuntimeStateWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder) (interactionRuntimeState, error) {
	if decoder == nil {
		return interactionRuntimeState{}, fmt.Errorf("skill: interaction: nil overworld decoder")
	}
	state := decoder.DecodeOverworld(reader)
	if state.NativeMapID > 0xff {
		return interactionRuntimeState{}, fmt.Errorf("skill: interaction: native map id %#04x exceeds current routing range", state.NativeMapID)
	}
	return interactionRuntimeState{
		Map:      uint8(state.NativeMapID),
		X:        state.X,
		Y:        state.Y,
		InBattle: state.InBattle,
	}, nil
}

func interactionStepWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder, tx, ty uint8) (world.Step, interactionRuntimeState, error) {
	live, err := interactionRuntimeStateWithDecoder(reader, decoder)
	if err != nil {
		return world.Step{}, interactionRuntimeState{}, err
	}
	step, ok := directionTo(live.X, live.Y, tx, ty)
	if !ok {
		return world.Step{}, live, fmt.Errorf("skill: interaction: tile (%d,%d) is not orthogonally adjacent to (%d,%d)", tx, ty, live.X, live.Y)
	}
	return step, live, nil
}
