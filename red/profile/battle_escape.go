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

	// redBattleMenuTileMapWidth is the Gen-I 20-column background tilemap, used
	// to recover the cursor's screen x from its tilemap offset.
	redBattleMenuTileMapWidth = 20

	// redBattleMenuCommandMax is wMaxMenuItem for the one-row, two-column
	// FIGHT/ITEM/PKMN/RUN command menu. The move menu reports the number of
	// moves instead, so this value identifies the menu that offers RUN.
	redBattleMenuCommandMax byte = 1
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

	// The command menu is identified by its own shape, not by the FIGHT text
	// alone. The command menu's entries stay in the tilemap while a submenu is
	// open, so the move menu shows a leftover FIGHT on row 14 above its own
	// ITEM RUN row while wTopMenuItemX/wCurrentMenuItem belong to the move
	// list. Matching that leftover made Flee drive the move menu:
	// run-1wsyy1f75ssxsheu3o4xpui4 reported the ordinary menu with column -1
	// (cursor offset 265 = row 13 col 5, inside the move panel) and failed
	// closed with "battle escape cursor stuck at col=-1 row=0, want RUN at
	// col=1 row=1" (triage:cfe63dc059a15aa5).
	//
	// wMaxMenuItem separates them: the command menu is one row of two columns
	// (max 1) while the move menu has as many rows as the active mon has moves.
	if mem.U8(sym.MaxMenuItem) != redBattleMenuCommandMax {
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

	// Read the column from where the cursor is actually drawn, not from
	// wTopMenuItemX. That byte is shared with the move menu (which draws at
	// x=5) and with the overworld menus, so a battle whose move menu has been
	// open leaves it at 5 even after FIGHT/ITEM/RUN is back on screen. The
	// cursor glyph's tilemap position is the positive screen evidence the other
	// cursor menus use: it is written only by the menu that owns input, and
	// every row of a column shares one screen x.
	column := -1
	if offset, ok := state.MenuCursorOffset(&mem); ok {
		switch cursorX := byte(offset % redBattleMenuTileMapWidth); cursorX {
		case leftX:
			column = 0
		case rightX:
			column = 1
		}
	}
	if column < 0 {
		// No drawn cursor to measure (a frame between the menu being drawn and
		// PlaceMenuCursor running). Fall back to the native byte.
		switch mem.U8(sym.TopMenuItemX) {
		case leftX:
			column = 0
		case rightX:
			column = 1
		}
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
