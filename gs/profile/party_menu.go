package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func (*Profile) DecodePartyMenu(reader game.MemoryReader) game.PartyMenuState {
	if reader == nil {
		return game.PartyMenuState{}
	}
	count := int(reader.Peek8(sym.PartyCount))
	if count <= 0 || count > 6 {
		return game.PartyMenuState{}
	}
	y, x, rows, cols, filter := gsMenuCursor(reader)
	if cols != 1 || rows != byte(count+1) || x != 1 ||
		y < 1 || int(y) > count+1 || filter != gen2PadA|gen2PadB {
		return game.PartyMenuState{}
	}
	text := gsScreenText(reader)
	inBattle := reader.Peek8(sym.BattleMode) != 0
	kind := game.PartyMenuKind("")
	switch {
	case inBattle && strings.Contains(text, "Which POK"):
		if battleBE16(reader, sym.BattleMonHP) == 0 {
			kind = game.PartyMenuForcedBattle
		} else {
			kind = game.PartyMenuVoluntaryBattle
		}
	case strings.Contains(text, "Use on which"):
		kind = game.PartyMenuItemUse
	case !inBattle && strings.Contains(text, "Choose a POK"):
		kind = game.PartyMenuFieldMove
	default:
		return game.PartyMenuState{}
	}
	return game.PartyMenuState{
		Visible: true,
		Kind:    kind,
		Cursor: game.MenuCursorState{
			Current: int(y) - 1,
			Max:     count - 1,
		},
	}
}

var _ game.PartyMenuDecoder = (*Profile)(nil)


// TMHMCompatiblePartySlots reads Gold/Silver's native ABLE/NOT ABLE labels.
// The game has already evaluated the species compatibility bitset by the time
// this screen is visible, so the executor can select a cartridge-confirmed
// recipient without reimplementing the ROM's compatibility table.
func (*Profile) TMHMCompatiblePartySlots(reader game.MemoryReader) []int {
	if reader == nil {
		return nil
	}
	state := (&Profile{}).DecodePartyMenu(reader)
	if !state.Visible || state.Kind != game.PartyMenuTMHMTeach {
		return nil
	}
	count := int(reader.Peek8(sym.PartyCount))
	if count > 6 {
		count = 6
	}
	out := make([]int, 0, count)
	for slot := 0; slot < count; slot++ {
		// PlacePartyMonTMHMCompatibility draws at hlcoord 12, 2 and then
		// advances by two tile rows. "ABLE" starts with tile $80 ('A');
		// "NOT ABLE" starts with $8d ('N').
		addr := sym.TileMap + uint16((2+2*slot)*20+12)
		if reader.Peek8(addr) == 0x80 {
			out = append(out, slot)
		}
	}
	return out
}
