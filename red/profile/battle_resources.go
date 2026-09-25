package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func (*Profile) DecodeBattleResources(reader game.MemoryReader) game.BattleResourcesState {
	if reader == nil {
		return game.BattleResourcesState{ActiveSlot: -1}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	party := state.DecodeParty(&mem)
	inventory := state.DecodeInventory(&mem)

	out := game.BattleResourcesState{
		InBattle:   state.DecodeBattle(&mem) != nil,
		ActiveSlot: int(mem.U8(sym.PlayerMonNumber)),
		Party:      make([]game.BattlePartyMon, 0, len(party.Mons)),
		Bag:        make([]game.InventoryItem, 0, len(inventory.Items)),
	}
	for _, mon := range party.Mons {
		projected := game.BattlePartyMon{
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
			projected.Moves[i] = game.BattlePartyMove{
				NativeMoveID: uint16(mon.Moves[i]),
				PP:           mon.PP[i],
			}
		}
		out.Party = append(out.Party, projected)
	}
	for _, item := range inventory.Items {
		out.Bag = append(out.Bag, game.InventoryItem{
			NativeItemID: uint16(item.ID),
			Quantity:     int(item.Quantity),
		})
	}
	return out
}
