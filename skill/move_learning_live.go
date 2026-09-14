package skill

import (
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// learningMonSnapshot returns the canonical move set used by Red's level-up
// learning code. During GainExperience the battle copy (wBattleMonMoves) can
// lag behind wPartyMons; the try-learn and forget menus operate on the party
// struct, so strategic replacement decisions must use that same source.
func learningMonSnapshot(mem *state.Mem, bs *state.BattleState) (moves [4]uint8, type1, type2 uint8, partySlot int) {
	partySlot = int(mem.U8(sym.PlayerMonNumber))
	party := state.DecodeParty(mem)
	if partySlot >= 0 && partySlot < len(party.Mons) {
		mon := party.Mons[partySlot]
		return mon.Moves, mon.Type1, mon.Type2, partySlot
	}

	// Defensive fallback for malformed/transitional party bookkeeping. Normal
	// move-learning paths always have a valid wPlayerMonNumber.
	if bs != nil {
		for i := range bs.Moves {
			moves[i] = bs.Moves[i].ID
		}
		return moves, bs.ActiveType1, bs.ActiveType2, partySlot
	}
	return moves, 0, 0, partySlot
}

func decideNaturalMoveFromPartyRAM(romData []byte, mem *state.Mem, bs *state.BattleState, offered uint8, blocked map[uint8]bool) (MoveLearningDecision, int) {
	moves, type1, type2, partySlot := learningMonSnapshot(mem, bs)
	return decideNaturalMove(romData, type1, type2, moves, offered, blocked), partySlot
}

func learningMoveID(mem *state.Mem, bs *state.BattleState, slot int) uint8 {
	moves, _, _, _ := learningMonSnapshot(mem, bs)
	if slot < 0 || slot >= len(moves) {
		return 0
	}
	return moves[slot]
}
