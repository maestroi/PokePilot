package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	safariBattleMenuMarker      = "THROW ROCK"
	safariBattleMenuLeftX  byte = 0x01
	safariBattleMenuRightX byte = 0x0d
)

// DecodeBattleEscapeMenu recognizes only fully rendered menus that offer RUN.
// Red's ordinary and Safari battle menus use different native X coordinates;
// both are projected onto the same semantic two-column grid.
func (*Profile) DecodeBattleEscapeMenu(reader game.MemoryReader) game.BattleEscapeMenuState {
	if reader == nil {
		return game.BattleEscapeMenuState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	if state.DecodeBattle(&mem) == nil {
		return game.BattleEscapeMenuState{}
	}

	text := state.ScreenText(&mem)
	kind := game.BattleEscapeMenuKind("")
	leftX, rightX := byte(0), byte(0)
	switch {
	case strings.Contains(text, battleMainMenuMarker):
		kind = game.BattleEscapeMenuOrdinary
		leftX, rightX = redBattleMenuLeftX, redBattleMenuRightX
	case strings.Contains(text, safariBattleMenuMarker):
		kind = game.BattleEscapeMenuSafari
		leftX, rightX = safariBattleMenuLeftX, safariBattleMenuRightX
	default:
		return game.BattleEscapeMenuState{}
	}

	column := -1
	switch mem.U8(sym.TopMenuItemX) {
	case leftX:
		column = 0
	case rightX:
		column = 1
	}
	return game.BattleEscapeMenuState{
		Visible: true,
		Kind:    kind,
		Cursor: game.BattleMenuPosition{
			Column: column,
			Row:    state.DecodeMenu(&mem).Current,
		},
	}
}

// BattleEscapeRunPosition maps RUN onto the semantic battle grid. The native
// cursor X differs between ordinary and Safari menus, but the intent does not.
func (*Profile) BattleEscapeRunPosition(kind game.BattleEscapeMenuKind) (game.BattleMenuPosition, bool) {
	switch kind {
	case game.BattleEscapeMenuOrdinary, game.BattleEscapeMenuSafari:
		return game.BattleMenuPosition{Column: 1, Row: 1}, true
	default:
		return game.BattleMenuPosition{}, false
	}
}
