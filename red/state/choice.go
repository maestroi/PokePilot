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

// menuCursorTile is the filled '▶' glyph PlaceMenuCursor writes at the menu
// cursor's location. $ED per charmap.asm:177. It has no entry in textChars, so
// ScreenText/DecodeTiles render it as a space; the cursor check must read the
// raw tile id from wTileMap instead.
const menuCursorTile = 0xED

// menuCursorSelectedTile is '▷' ($EC per charmap.asm:176), the UNFILLED cursor
// PlaceUnfilledArrowMenuCursor writes over the same location. The START menu
// calls it as soon as a button is pressed, before it branches on which button:
//
//	.buttonPressed
//	    call PlaceUnfilledArrowMenuCursor
//	    ld a, [wCurrentMenuItem] ...
//
// so a menu that is mid-selection — or a save state captured at that instant —
// shows $EC where a waiting menu shows $ED. Both are the cursor glyph and both
// mean a live menu; accepting only the filled one was how the START menu left
// by UseEvolutionItem still read as "no menu".
const menuCursorSelectedTile = 0xEC

// menuCursorOffset returns the wTileMap offset the ROM currently holds for the
// menu cursor, and whether that location is inside the 20x18 tilemap.
//
// Every cursor menu publishes its cursor through wMenuCursorLocation:
// PlaceMenuCursor (pokered/home/window.asm) stores the tilemap address as it
// draws, and HandleMenuInput calls it once per input-loop iteration, so the
// value is live whenever a menu is waiting for input. Read it instead of
// assuming the cursor sits on the menu's FIRST item: PlaceMenuCursor walks
// down from (wTopMenuItemY, wTopMenuItemX) once per wCurrentMenuItem, and once
// more per item when BIT_DOUBLE_SPACED_MENU is set. The START menu is
// double-spaced with wTopMenuItemY=2, so ITEM selected (wCurrentMenuItem=2)
// draws its cursor at (11,6) while (11,2) still holds the box border.
func menuCursorOffset(m *Mem) (int, bool) {
	cursor := uint16(m.U8(sym.MenuCursorLocation)) | uint16(m.U8(sym.MenuCursorLocation+1))<<8
	if cursor < sym.TileMap {
		return 0, false
	}
	offset := int(cursor - sym.TileMap)
	if offset < 0 || offset >= sym.TileMapLen {
		return 0, false
	}
	return offset, true
}

// menuCursorDrawn reports whether a cursor glyph — filled or unfilled — is
// actually drawn at the location the ROM published. This is the positive,
// screen-level evidence that separates a live menu from stale cursor bytes.
func menuCursorDrawn(m *Mem) bool {
	tile, ok := menuCursorGlyph(m)
	return ok && (tile == menuCursorTile || tile == menuCursorSelectedTile)
}

// menuCursorGlyph returns the raw tile at the ROM-published cursor location.
func menuCursorGlyph(m *Mem) (uint8, bool) {
	offset, ok := menuCursorOffset(m)
	if !ok {
		return 0, false
	}
	return m.Slice(sym.TileMap, sym.TileMapLen)[offset], true
}

// DecodeTwoOptionMenu reports the live two-option prompt, or nil when none
// is up. Three conditions, all positive, all from live state:
//
//  1. The screen holds drawn text: wFontLoaded != 0 in the overworld, or
//     wIsInBattle != 0 in a battle. wFontLoaded is MEASURED to stay 0 for
//     an entire battle (docs/archive/SLICE3-PLAN.md Addendum 2): DisplayTwoOptionMenu
//     does not set it, and battle text does not go through the overworld
//     text engine, so the wild-battle "Use next #MON?" prompt (core.asm
//     DoUseNextMonDialogue) would otherwise be undecodable. The staleness
//     guard is condition 3, which holds in both contexts.
//  2. wMaxMenuItem == 1 — the highest valid menu index is 1, the shape
//     DisplayTwoOptionMenu writes.
//  3. A cursor glyph is actually
//     drawn where the ROM recorded it. This is the condition that kills the
//     stale-RAM false positive: stale coordinates point at a tile the game
//     never drew a cursor on, so the check fails closed. It must read
//     wMenuCursorLocation rather than the top-item coordinates, or a prompt
//     whose cursor is on the second option (wCurrentMenuItem == 1) reads as
//     "no prompt" and is then misclassified as ordinary dialogue.
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
	// Only the FILLED cursor is a prompt waiting for an answer. Two-option
	// menus never draw '▷' while waiting; callers draw it after
	// HandleMenuInput has already returned a choice. The item menu's USE/TOSS
	// box (start_sub_menus.asm .choseItem) is the measured case: it leaves
	// wMaxMenuItem=1 and '▷' on USE while the TM's "Teach X?" text prints,
	// and reading that as a live prompt answered nothing
	// (run-22ahrk9pflcilu3jxq9xt37x6, TeachTMHM).
	if tile, ok := menuCursorGlyph(m); !ok || tile != menuCursorTile {
		return nil
	}
	return &TwoOptionMenu{Index: int(m.U8(sym.CurrentMenuItem))}
}
