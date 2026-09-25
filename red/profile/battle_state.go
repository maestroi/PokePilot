package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

// DecodeBattleState keeps the Gen-I battle RAM layout behind the profile
// boundary while returning the portable live combat contract.
func (*Profile) DecodeBattleState(reader game.MemoryReader) (game.BattleState, bool) {
	if reader == nil {
		return game.BattleState{}, false
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	b := state.DecodeBattle(&mem)
	if b == nil {
		return game.BattleState{}, false
	}
	return *b, true
}

func (*Profile) DecodeBattleResult(reader game.MemoryReader) game.BattleResult {
	if reader == nil {
		return game.BattleWon
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	return state.DecodeBattleResult(&mem)
}
