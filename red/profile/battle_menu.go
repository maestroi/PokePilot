package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	redBattleMenuLeftX  byte = 0x09
	redBattleMenuRightX byte = 0x0f
)

// DecodeBattleMainMenu keeps Red/Blue's tile marker and cursor RAM layout
// behind the profile boundary.
func (*Profile) DecodeBattleMainMenu(reader game.MemoryReader) game.BattleMainMenuState {
	if reader == nil {
		return game.BattleMainMenuState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	if state.DecodeBattle(&mem) == nil || !strings.Contains(state.ScreenText(&mem), "FIGHT") {
		return game.BattleMainMenuState{}
	}

	column := -1
	switch mem.U8(sym.TopMenuItemX) {
	case redBattleMenuLeftX:
		column = 0
	case redBattleMenuRightX:
		column = 1
	}
	return game.BattleMainMenuState{
		Visible: true,
		Cursor: game.BattleMenuPosition{
			Column: column,
			Row:    state.DecodeMenu(&mem).Current,
		},
	}
}

// BattleMainMenuEntryPosition maps semantic actions to Red/Blue's 2x2 battle
// command grid. Gen II is free to expose a different ordering or shape.
func (*Profile) BattleMainMenuEntryPosition(entry game.BattleMenuEntry) (game.BattleMenuPosition, bool) {
	switch entry {
	case game.BattleMenuFight:
		return game.BattleMenuPosition{Column: 0, Row: 0}, true
	case game.BattleMenuItems:
		return game.BattleMenuPosition{Column: 0, Row: 1}, true
	case game.BattleMenuPokemon:
		return game.BattleMenuPosition{Column: 1, Row: 0}, true
	case game.BattleMenuRun:
		return game.BattleMenuPosition{Column: 1, Row: 1}, true
	default:
		return game.BattleMenuPosition{}, false
	}
}
