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

	// gen2BattleTypeTutorial is BATTLETYPE_TUTORIAL: the Route 29 Dude's
	// catching demo.
	gen2BattleTypeTutorial byte = 3
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
	// The demo draws a command menu the ROM steers itself, with no player
	// mon loaded: pressing FIGHT on it never opens a move list
	// (run-1lf849uc4815y2tkvu2odh07vc, triage:3956fc7778cd88cf).
	if reader.Peek8(sym.BattleType) == gen2BattleTypeTutorial {
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
func (*Profile) DecodeMenuCursor(reader game.MemoryReader) game.MenuCursorState {
	if reader == nil {
		return game.MenuCursorState{}
	}
	y, _, rows, cols, filter := gsMenuCursor(reader)
	if reader.Peek8(sym.BattleMode) != 0 &&
		reader.Peek8(sym.MoveSelectionMenuType) == 0 &&
		cols == 1 && rows >= 1 && rows <= 4 &&
		filter == gen2BattleMoveFilter &&
		(strings.Contains(gsScreenText(reader), "TYPE/") || gsMovePanelShowsDisabled(reader)) {
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
	// Gen-II's menu RAM persists after a menu closes, so the 2x1 A|B shape
	// alone is not proof the box is on screen; reading it as a live prompt
	// makes the answer postcondition in selectTwoOption fail closed on
	// stale state (run-11dd5ya1qev0ry, triage:e1ef45bf25e3b946). The rendered
	// YES/NO words are the positive liveness proof, the same role Red's
	// drawn cursor glyph plays.
	text := gsScreenText(reader)
	if !strings.Contains(text, "YES") || !strings.Contains(text, "NO") {
		return game.TwoOptionState{}, false
	}
	return game.TwoOptionState{Current: int(y) - 1}, true
}

// DecodeStartMenu recognizes the normal Gen-II overworld START menu from
// rendered labels plus its live vertical cursor shape. Menu RAM persists after
// ExitMenu, so RAM alone is never treated as visibility proof.
func (*Profile) DecodeStartMenu(reader game.MemoryReader) game.StartMenuState {
	if reader == nil {
		return game.StartMenuState{}
	}
	inBattle := reader.Peek8(sym.BattleMode) != 0
	text := gsScreenText(reader)
	visible := strings.Contains(text, "PACK") &&
		strings.Contains(text, "SAVE") &&
		strings.Contains(text, "OPTION") &&
		strings.Contains(text, "EXIT")
	y, _, rows, cols, _ := gsMenuCursor(reader)
	ready := visible && !inBattle && cols == 1 && rows >= 6 && y >= 1 && y <= rows
	current := int(y) - 1
	if current < 0 {
		current = 0
	}
	max := int(rows) - 1
	if max < 0 {
		max = 0
	}
	return game.StartMenuState{
		Visible:  visible,
		Ready:    ready,
		InBattle: inBattle,
		Cursor:   game.MenuCursorState{Current: current, Max: max},
	}
}

// StartMenuEntryIndex maps the two entries generic execution currently needs.
// In Gen II the optional Pokédex precedes POKéMON, while PACK immediately
// follows POKéMON in ordinary overworld play. Pokégear is inserted later and
// therefore does not shift either index.
func (*Profile) StartMenuEntryIndex(reader game.MemoryReader, entry game.StartMenuEntry) (int, bool) {
	if reader == nil {
		return 0, false
	}
	pokedexOffset := 0
	if reader.Peek8(sym.StatusFlags)&1 != 0 { // STATUSFLAGS_POKEDEX_F
		pokedexOffset = 1
	}
	partyPresent := reader.Peek8(sym.PartyCount) > 0
	switch entry {
	case game.StartMenuPokemon:
		if !partyPresent {
			return 0, false
		}
		return pokedexOffset, true
	case game.StartMenuItems:
		// Bug Contest/link variants can omit PACK. Only advertise it when the
		// currently rendered START menu positively contains the entry.
		if !strings.Contains(gsScreenText(reader), "PACK") {
			return 0, false
		}
		index := pokedexOffset
		if partyPresent {
			index++
		}
		return index, true
	default:
		return 0, false
	}
}
