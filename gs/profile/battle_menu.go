package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func (*Profile) DecodeBattleMainMenu(reader game.MemoryReader) game.BattleMainMenuState {
	if !gen2BattleMainMenuVisible(reader) {
		return game.BattleMainMenuState{}
	}
	return game.BattleMainMenuState{
		Visible: true,
		Cursor: game.BattleMenuPosition{
			Column: max(0, int(reader.Peek8(sym.MenuCursorX))-1),
			Row:    max(0, int(reader.Peek8(sym.MenuCursorY))-1),
		},
	}
}

func (*Profile) BattleMainMenuEntryPosition(entry game.BattleMenuEntry) (game.BattleMenuPosition, bool) {
	switch entry {
	case game.BattleMenuFight:
		return game.BattleMenuPosition{Column: 0, Row: 0}, true
	case game.BattleMenuPokemon:
		return game.BattleMenuPosition{Column: 1, Row: 0}, true
	case game.BattleMenuItems:
		return game.BattleMenuPosition{Column: 0, Row: 1}, true
	case game.BattleMenuRun:
		return game.BattleMenuPosition{Column: 1, Row: 1}, true
	default:
		return game.BattleMenuPosition{}, false
	}
}

var _ game.BattleMenuDecoder = (*Profile)(nil)
