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
	case inBattle && strings.Contains(text, "Use on which"):
		kind = game.PartyMenuItemUse
	case inBattle && strings.Contains(text, "CANCEL"):
		// The battle party list renders a CANCEL row. The bottom prompt is
		// "Which  ?" — no species word — so the list row, not the prompt, is
		// the stable identity of the surface (measured on the live stall
		// screen, run-11dd5ya1qev0ry, triage:e1ef45bf25e3b946).
		if battleBE16(reader, sym.BattleMonHP) == 0 {
			kind = game.PartyMenuForcedBattle
		} else {
			kind = game.PartyMenuVoluntaryBattle
		}
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
