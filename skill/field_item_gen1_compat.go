package skill

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	itemListMenuID = 3

	itemEther     uint8 = 0x50
	itemMaxEther  uint8 = 0x51
	itemElixer    uint8 = 0x52
	itemMaxElixer uint8 = 0x53
)

// useTossPrompt remains for Gen-I callers outside the migrated field-item lane.
// Generic UseFieldItem/UseRepel use game.FieldItemDecoder instead.
func useTossPrompt(mem *state.Mem) *state.TwoOptionMenu {
	p := state.DecodeTwoOptionMenu(mem)
	if p == nil || mem.U8(sym.TopMenuItemY) != 11 || mem.U8(sym.TopMenuItemX) != 14 {
		return nil
	}
	return p
}

func isSingleMovePPRestore(item uint8) bool {
	return item == itemEther || item == itemMaxEther
}

func isPPRestoreItem(item uint8) bool {
	return item >= itemEther && item <= itemMaxElixer
}

func gen1FieldItemMon(mon state.Mon) game.FieldItemPartyMon {
	out := game.FieldItemPartyMon{
		NativeSpeciesID: uint16(mon.Species),
		Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP,
		Status: mon.StatusName(),
	}
	for i := range mon.Moves {
		out.Moves[i] = uint16(mon.Moves[i])
		out.PP[i] = mon.PP[i]
	}
	return out
}

func ppRestoreMoveSlot(mon state.Mon) (int, bool) {
	return ppRestoreMoveSlotState(gen1FieldItemMon(mon))
}

func fieldItemHadEffect(before, after state.Mon) bool {
	return fieldItemHadEffectState(gen1FieldItemMon(before), gen1FieldItemMon(after))
}
