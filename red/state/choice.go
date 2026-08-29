package state

import "github.com/maestroi/pokepilot/red/sym"

// Two-option prompts ("Shall we heal your Pokemon?", "Do you want to take
// this Pokemon?") are drawn by DisplayTwoOptionMenu
// (pokered/engine/menus/text_box.asm): it writes wMaxMenuItem = 1, stores
// the caller-supplied b/c in wTopMenuItemY (0xCC24) and wTopMenuItemX
// (0xCC25), and PlaceMenuCursor (pokered/home/window.asm) draws the
// '▶' cursor glyph ($ED, pokered/constants/charmap.asm:177) at
// (wTopMenuItemY, wTopMenuItemX).
//
// The facts below were measured on the real ROM, 2026-08-28. Do not
// re-derive them.
//
// Fact 1: the old yesNoMenuUp predicate (FontLoaded != 0 &&
// wMaxMenuItem == 1) had a live false positive. Measured immediately after
// skill.Heal answered the nurse, while ordinary heal text was on screen:
//
//	FontLoaded=0x01  wMaxMenuItem=1 (STALE)  wTopMenuItemY=8 wTopMenuItemX=12 (STALE)
//	tile at (row 8, col 12) = 0x01   <- not a cursor
//
// Both old conditions held, so it returned TRUE on plain prose. The
// dangerous staleness is in wMaxMenuItem and wTopMenuItem — exactly what
// the old predicate read.
//
// Fact 2: the menu is not at a fixed position. DisplayTwoOptionMenu stores
// the caller's b/c registers, so every prompt is drawn wherever its call
// site puts it. A pinned column would be right for one prompt and wrong
// for the next; the position must be read from RAM.
//
// Fact 3: the cursor tile fails closed on stale coordinates. Measured at
// the nurse's live prompt:
//
//	FontLoaded=0x01  wTextBoxID=0x14  wMaxMenuItem=1
//	wTopMenuItemY=8  wTopMenuItemX=12
//	tile at (row 8, col 12) = 0xED    <- the cursor, present
//
// Measured in the plain overworld, nothing on screen:
//
//	wTopMenuItemY=12 wTopMenuItemX=5 wMaxMenuItem=3   (stale START menu values)
//	tile at (row 12, col 5) = 0x0b    <- not a cursor
//
// Stale RAM points at a tile the game never drew a cursor on, so checking
// the tile the coordinates point at is the safety property.
//
// Fact 4: the options are not always YES and NO. The nurse's prompt reads
// HEAL / CANCEL, and the options are double-spaced (hUILayoutFlags
// BIT_DOUBLE_SPACED_MENU): the second option sits two rows below the
// first, not one. The strings come from TwoOptionMenuStrings indexed by
// wTwoOptionMenuID and differ per prompt, so nothing here matches option
// text.

// TwoOptionMenu is a live two-option prompt: the game is asking a yes/no
// shaped question and the cursor is drawn.
type TwoOptionMenu struct {
	Index int // wCurrentMenuItem: 0 or 1, which option the cursor is on
}

// menuCursorTile is the '▶' glyph PlaceMenuCursor writes at
// (wTopMenuItemY, wTopMenuItemX). $ED per charmap.asm:177. It has no entry
// in textChars, so ScreenText/DecodeTiles render it as a space; the cursor
// check must read the raw tile id from wTileMap instead.
const menuCursorTile = 0xED

// DecodeTwoOptionMenu reports the live two-option prompt, or nil when none
// is up. Three conditions, all positive, all from live state:
//
//  1. The screen holds drawn text: wFontLoaded != 0 in the overworld, or
//     wIsInBattle != 0 in a battle. wFontLoaded is MEASURED to stay 0 for
//     an entire battle (docs/SLICE3-PLAN.md Addendum 2): DisplayTwoOptionMenu
//     does not set it, and battle text does not go through the overworld
//     text engine, so the wild-battle "Use next #MON?" prompt (core.asm
//     DoUseNextMonDialogue) would otherwise be undecodable. The staleness
//     guard is condition 3, which holds in both contexts.
//  2. wMaxMenuItem == 1 — the highest valid menu index is 1, the shape
//     DisplayTwoOptionMenu writes.
//  3. wTileMap[wTopMenuItemY*20 + wTopMenuItemX] == $ED — the cursor glyph
//     is actually drawn where the coordinates say it is. This is the
//     condition that kills the stale-RAM false positive: stale
//     coordinates point at a tile the game never drew a cursor on, so the
//     check fails closed.
//
// The tile is read raw from wTileMap as 20-wide rows, never via
// ScreenText or DecodeTiles: textChars has no entry for $ED, so the cursor
// would render as a space and the check could never see it.
func DecodeTwoOptionMenu(m *Mem) *TwoOptionMenu {
	if m.U8(sym.FontLoaded) == 0 && m.U8(sym.IsInBattle) == 0 {
		return nil
	}
	if m.U8(sym.MaxMenuItem) != 1 {
		return nil
	}
	y := int(m.U8(sym.TopMenuItemY))
	x := int(m.U8(sym.TopMenuItemX))
	if y >= 18 || x >= 20 {
		return nil
	}
	if m.Slice(sym.TileMap, sym.TileMapLen)[y*20+x] != menuCursorTile {
		return nil
	}
	return &TwoOptionMenu{Index: int(m.U8(sym.CurrentMenuItem))}
}
