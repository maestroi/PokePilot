package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gen2PadA             byte = 0x01
	gen2PadB             byte = 0x02
	gen2PadSelect        byte = 0x04
	gen2PadUp            byte = 0x40
	gen2PadDown          byte = 0x80
	gen2BattleMoveFilter      = gen2PadA | gen2PadB | gen2PadSelect | gen2PadUp | gen2PadDown
)

var (
	_ game.BattleMenuDecoder = (*Profile)(nil)
	_ game.MenuDecoder       = (*Profile)(nil)
)

func gsMenuCursor(reader game.MemoryReader) (y, x, rows, cols, filter byte) {
	if reader == nil {
		return
	}
	return reader.Peek8(sym.MenuCursorY),
		reader.Peek8(sym.MenuCursorX),
		reader.Peek8(sym.TwoDMenuNumRows),
		reader.Peek8(sym.TwoDMenuNumCols),
		reader.Peek8(sym.MenuJoypadFilter)
}

func (*Profile) DecodeBattleMainMenu(reader game.MemoryReader) game.BattleMainMenuState {
	if reader == nil || reader.Peek8(sym.BattleMode) == 0 {
		return game.BattleMainMenuState{}
	}
	y, x, rows, cols, filter := gsMenuCursor(reader)
	if rows != 2 || cols != 2 || filter != gen2PadA ||
		y < 1 || y > 2 || x < 1 || x > 2 ||
		!strings.Contains(gsScreenText(reader), "FIGHT") {
		return game.BattleMainMenuState{}
	}
	return game.BattleMainMenuState{
		Visible: true,
		Cursor: game.BattleMenuPosition{
			Column: int(x) - 1,
			Row:    int(y) - 1,
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

// DecodeMenuCursor normalizes Gen-II's vertical menus for the shared cursor
// driver. The ordinary battle move list deliberately preserves its native
// 1-based selection index because skill.Battle's move slot contract is
// slot+1 in both supported generations.
func (p *Profile) DecodeMenuCursor(reader game.MemoryReader) game.MenuCursorState {
	if reader == nil {
		return game.MenuCursorState{}
	}
	if start := p.DecodeStartMenu(reader); start.Visible {
		return start.Cursor
	}
	y, _, rows, cols, filter := gsMenuCursor(reader)
	if reader.Peek8(sym.BattleMode) != 0 &&
		reader.Peek8(sym.MoveSelectionMenuType) == 0 &&
		cols == 1 && rows >= 1 && rows <= 4 &&
		filter == gen2BattleMoveFilter &&
		strings.Contains(gsScreenText(reader), "TYPE/") {
		return game.MenuCursorState{Current: int(y), Max: int(rows)}
	}
	if rows == 2 && cols == 1 && filter == gen2PadA|gen2PadB {
		current := int(y) - 1
		if current < 0 {
			current = 0
		}
		return game.MenuCursorState{Current: current, Max: 1}
	}
	current := int(y) - 1
	if current < 0 {
		current = 0
	}
	max := int(rows) - 1
	if max < 0 {
		max = 0
	}
	return game.MenuCursorState{Current: current, Max: max}
}

func (*Profile) DecodeTwoOption(reader game.MemoryReader) (game.TwoOptionState, bool) {
	if reader == nil {
		return game.TwoOptionState{}, false
	}
	y, x, rows, cols, filter := gsMenuCursor(reader)
	if rows != 2 || cols != 1 || filter != gen2PadA|gen2PadB ||
		y < 1 || y > 2 || x != 1 {
		return game.TwoOptionState{}, false
	}
	return game.TwoOptionState{Current: int(y) - 1}, true
}

const (
	gen2StartMenuPokemon byte = 1
	gen2StartMenuPack    byte = 2
)

// DecodeStartMenu uses the retail menu-item list rather than assuming a fixed
// row ordering. Gold/Silver insert POKEDEX and POKEGEAR dynamically, so the
// list is the stable semantic source while rendered PACK/OPTION text proves
// the union-backed bytes belong to the live START menu.
func (*Profile) DecodeStartMenu(reader game.MemoryReader) game.StartMenuState {
	if reader == nil {
		return game.StartMenuState{}
	}
	inBattle := reader.Peek8(sym.BattleMode) != 0
	out := game.StartMenuState{InBattle: inBattle}
	if inBattle {
		return out
	}
	count := int(reader.Peek8(sym.MenuItemsList))
	if count < 2 || count > 9 {
		return out
	}
	text := gsScreenText(reader)
	if !strings.Contains(text, "PACK") || !strings.Contains(text, "OPTION") {
		return out
	}
	cursor := int(reader.Peek8(sym.MenuCursorPosition)) - 1
	if cursor < 0 || cursor >= count {
		return out
	}
	out.Visible = true
	out.Ready = true
	out.Cursor = game.MenuCursorState{Current: cursor, Max: count - 1}
	return out
}

func (*Profile) StartMenuEntryIndex(reader game.MemoryReader, entry game.StartMenuEntry) (int, bool) {
	if reader == nil {
		return 0, false
	}
	want := byte(0xff)
	switch entry {
	case game.StartMenuPokemon:
		want = gen2StartMenuPokemon
	case game.StartMenuItems:
		want = gen2StartMenuPack
	default:
		return 0, false
	}
	count := int(reader.Peek8(sym.MenuItemsList))
	if count < 1 || count > 9 {
		return 0, false
	}
	for i := 0; i < count; i++ {
		if reader.Peek8(sym.MenuItemsList+1+uint16(i)) == want {
			return i, true
		}
	}
	return 0, false
}
