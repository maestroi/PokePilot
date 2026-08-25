package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

// startMenuMax is the Start menu's wMaxMenuItem in the fixture state: no
// pokedex, so six items (POKéMON, ITEM, player name, SAVE, OPTIONS, EXIT).
// wMaxMenuItem holds the item count, 6, so the valid cursor indices are 0..5.
// Ground truth from the decompilation (DrawStartMenu) and confirmed by
// stepping the cursor: it reaches 5 and wraps to 0, never reading 6.
const startMenuMax = 6

// bagListMenuID is wListMenuID for the Start menu's ITEM submenu (the bag).
// It is the positive identifier that index 1 (ITEM) was selected: the party
// submenu (index 0) is not a list menu and leaves wListMenuID at 0.
const bagListMenuID = 3 // ITEMLISTMENU

// TestSelectMenuItemStartMenu drives the real Start menu from the overworld:
// open it, assert the cursor shape, select index 1 step-and-verify, and close
// it with B. Every expectation is read from game RAM (wCurrentMenuItem,
// wMaxMenuItem, wListMenuID), never from pixels or press counts.
//
// Ground truth (decompilation + probing this ROM): index 0 = POKéMON,
// index 1 = ITEM. A on ITEM opens the bag list menu; with the fixture's empty
// bag it reports {Current:0 Max:0} and wListMenuID = ITEMLISTMENU. B there
// redraws the Start menu with the cursor restored to where A was pressed
// (wBattleAndStartSavedMenuItem).
func TestSelectMenuItemStartMenu(t *testing.T) {
	e := loadFixture(t)

	// Open the Start menu from the overworld. FontLoaded going non-zero is
	// necessary but not sufficient: the text-box font loads before
	// DrawStartMenu writes the cursor, and the menu RAM holds stale values
	// from earlier menus. wMaxMenuItem is the last write in DrawStartMenu,
	// so it marks the menu as fully drawn.
	e.Tap(emu.Start, 3, 7)
	if _, err := e.StepUntil(60, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0
	}); err != nil {
		var mem state.Mem
		state.Snapshot(e, &mem)
		t.Fatalf("Start menu did not open: wFontLoaded=%#04x wJoyIgnore=%#04x at map=%#04x (%d,%d)",
			mem.U8(sym.FontLoaded), mem.U8(sym.JoyIgnore), mem.U8(sym.CurMap),
			mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
	if _, err := e.StepUntil(60, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == startMenuMax
	}); err != nil {
		var mem state.Mem
		state.Snapshot(e, &mem)
		t.Fatalf("Start menu did not finish drawing: wFontLoaded=%#04x wCurrentMenuItem=%#04x wMaxMenuItem=%#04x",
			mem.U8(sym.FontLoaded), mem.U8(sym.CurrentMenuItem), mem.U8(sym.MaxMenuItem))
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	menu := state.DecodeMenu(&mem)
	if menu.Max <= 0 {
		t.Fatalf("Start menu Max = %d, want > 0 (wCurrentMenuItem=%#04x wMaxMenuItem=%#04x)",
			menu.Max, mem.U8(sym.CurrentMenuItem), mem.U8(sym.MaxMenuItem))
	}
	if menu.Current != 0 {
		t.Fatalf("fresh Start menu Current = %d, want 0 (wBattleAndStartSavedMenuItem=%#04x)",
			menu.Current, mem.U8(0xCC2D))
	}

	// Out of range: an error, and nothing changes (DESIGN.md 3.2b: a failure
	// assertion must also assert nothing changed). Valid indices are 0..Max-1,
	// so -1 and Max are the two boundary violations.
	for _, bad := range []int{-1, menu.Max} {
		if err := skill.SelectMenuItem(e, bad); err == nil {
			t.Fatalf("SelectMenuItem(%d) = nil, want out-of-range error (max %d)", bad, menu.Max)
		}
		state.Snapshot(e, &mem)
		if got := state.DecodeMenu(&mem); got.Current != menu.Current || mem.U8(sym.FontLoaded) == 0 {
			t.Fatalf("SelectMenuItem(%d) changed state: Current=%d wFontLoaded=%#04x; want Current=%d and the menu still open",
				bad, got.Current, mem.U8(sym.FontLoaded), menu.Current)
		}
	}

	// Select index 1. SelectMenuItem's contract: the cursor is asserted to be
	// at 1 before A is pressed.
	if err := skill.SelectMenuItem(e, 1); err != nil {
		t.Fatalf("SelectMenuItem(1): %v", err)
	}

	// A fired the index-1 action (ITEM): the bag list menu is open. Wait for
	// wListMenuID to become ITEMLISTMENU, the positive identifier that the
	// cursor was at 1 (A on index 0 opens the party submenu, which is not a
	// list menu).
	if _, err := e.StepUntil(60, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == bagListMenuID
	}); err != nil {
		var mem state.Mem
		state.Snapshot(e, &mem)
		t.Fatalf("item menu did not open after A: wFontLoaded=%#04x wCurrentMenuItem=%#04x wMaxMenuItem=%#04x wListMenuID=%#04x",
			mem.U8(sym.FontLoaded), mem.U8(sym.CurrentMenuItem), mem.U8(sym.MaxMenuItem), mem.U8(sym.ListMenuID))
	}
	state.Snapshot(e, &mem)
	if mem.U8(sym.FontLoaded) == 0 {
		t.Fatal("menu closed after SelectMenuItem(1), want the item menu open")
	}
	if got := state.DecodeMenu(&mem); got.Current != 0 || got.Max != 0 {
		t.Fatalf("after SelectMenuItem(1): menu = %+v, want the empty bag list menu {Current:0 Max:0}", got)
	}

	// Close the item menu with B: the Start menu is redrawn with the cursor
	// restored to where A was pressed. Current == 1 is the positive assertion
	// that the cursor had reached 1 before A. The list menu's input path needs
	// the joypad state to settle after the A press, so let it settle and use a
	// longer hold than the overworld taps.
	e.StepFrames(20)
	e.Tap(emu.B, 8, 10)
	if _, err := e.StepUntil(120, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == menu.Max
	}); err != nil {
		var mem state.Mem
		state.Snapshot(e, &mem)
		t.Fatalf("Start menu did not reappear after B: wFontLoaded=%#04x wCurrentMenuItem=%#04x wMaxMenuItem=%#04x wListMenuID=%#04x",
			mem.U8(sym.FontLoaded), mem.U8(sym.CurrentMenuItem), mem.U8(sym.MaxMenuItem), mem.U8(sym.ListMenuID))
	}
	state.Snapshot(e, &mem)
	if got := state.DecodeMenu(&mem); got.Current != 1 {
		t.Fatalf("Start menu cursor = %d after B, want 1 (where A was pressed)", got.Current)
	}

	// Close the Start menu with B: the player is controllable again.
	e.StepFrames(20)
	e.Tap(emu.B, 8, 10)
	if _, err := e.StepUntil(120, func(m *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return state.Controllable(&mem)
	}); err != nil {
		var mem state.Mem
		state.Snapshot(e, &mem)
		t.Fatalf("player not controllable after closing the menu: wFontLoaded=%#04x wJoyIgnore=%#04x",
			mem.U8(sym.FontLoaded), mem.U8(sym.JoyIgnore))
	}
}
