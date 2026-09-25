package skill

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// Gen-I compatibility wrappers keep existing Red strategy tests/callers
// source-stable while Battle itself consumes profile-projected resources.
func chooseTacticalSwitch(romData []byte, mem *state.Mem, b state.BattleState) switchDecision {
	return chooseTacticalSwitchState(romData, gen1BattleResourcesFromMem(mem), b)
}

func chooseTrainingCarrySwitch(romData []byte, mem *state.Mem, b state.BattleState, minLevel uint8) switchDecision {
	return chooseTrainingCarrySwitchState(romData, gen1BattleResourcesFromMem(mem), b, minLevel)
}

func bestReplacementSlot(romData []byte, mem *state.Mem, b state.BattleState) (int, switchEvaluation) {
	return bestReplacementSlotState(romData, gen1BattleResourcesFromMem(mem), b)
}

func gen1BattleResourcesFromMem(mem *state.Mem) game.BattleResourcesState {
	if mem == nil {
		return game.BattleResourcesState{ActiveSlot: -1}
	}
	party := state.DecodeParty(mem)
	out := game.BattleResourcesState{
		InBattle:   state.DecodeBattle(mem) != nil,
		ActiveSlot: int(mem.U8(sym.PlayerMonNumber)),
		Party:      make([]game.BattlePartyMon, 0, len(party.Mons)),
	}
	for _, mon := range party.Mons {
		p := game.BattlePartyMon{
			NativeSpeciesID: uint16(mon.Species),
			Level:           mon.Level,
			HP:              mon.HP,
			MaxHP:           mon.MaxHP,
			Status:          mon.StatusName(),
			Type1:           uint16(mon.Type1),
			Type2:           uint16(mon.Type2),
			Attack:          mon.Attack,
			Defense:         mon.Defense,
			Speed:           mon.Speed,
			Special:         mon.Special,
			SpecialAttack:   mon.Special,
			SpecialDefense:  mon.Special,
		}
		for i := range mon.Moves {
			p.Moves[i] = game.BattlePartyMove{NativeMoveID: uint16(mon.Moves[i]), PP: mon.PP[i]}
		}
		out.Party = append(out.Party, p)
	}
	return out
}
