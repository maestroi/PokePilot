package skill

import (
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// naturalMoveLearner returns the party member Red's LearnMove routine is
// currently processing. This is not necessarily the active battle Pokémon:
// experience is awarded party member by party member after a KO, and a mon
// can be asked to learn a move while another party member is active.
func naturalMoveLearner(mem *state.Mem) (state.Mon, int, bool) {
	if mem == nil {
		return state.Mon{}, -1, false
	}
	party := state.DecodeParty(mem)
	slot := int(mem.U8(sym.WhichPokemon))
	if slot < 0 || slot >= len(party.Mons) {
		return state.Mon{}, slot, false
	}
	return party.Mons[slot], slot, true
}

func naturalMoveDecisionForLearner(mem *state.Mem, romData []byte, offered uint8, blocked map[uint8]bool) (MoveLearningDecision, int, bool) {
	mon, slot, ok := naturalMoveLearner(mem)
	if !ok {
		return MoveLearningDecision{}, slot, false
	}
	return decideNaturalMove(romData, mon.Type1, mon.Type2, mon.Moves, offered, blocked), slot, true
}
