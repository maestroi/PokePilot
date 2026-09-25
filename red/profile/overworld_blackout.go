package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const gen1BattleOverOrBlackoutBit = 1 << 5

// DecodeOverworldBlackout keeps Gen-I poison-blackout, party-faint, and saved
// respawn-map RAM details behind the Red profile boundary.
func (*Profile) DecodeOverworldBlackout(reader game.MemoryReader) game.OverworldBlackoutState {
	if reader == nil {
		return game.OverworldBlackoutState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])

	party := state.DecodeParty(&mem)
	allFainted := len(party.Mons) > 0
	for _, mon := range party.Mons {
		if !mon.Fainted() {
			allFainted = false
			break
		}
	}

	// wStatusFlags4's bit is shared by normal battle settlement and blackout.
	// Requiring an actually fainted party turns it into the semantic
	// out-of-battle blackout transition Travel needs after poison dialogue.
	inProgress := mem.U8(sym.StatusFlags4)&gen1BattleOverOrBlackoutBit != 0 && allFainted

	return game.OverworldBlackoutState{
		BlackoutInProgress: inProgress,
		PartyAllFainted:     allFainted,
		RespawnNativeMapID: uint16(mem.U8(sym.LastBlackoutMap)),
	}
}
