package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeDialogueClosed(t *testing.T) {
	var m Mem
	m[sym.FontLoaded] = 0
	// Stale RAM from a previous menu/text box must not be read as open.
	m[sym.TextBoxID] = 0x2A

	if d := DecodeDialogue(&m); d != nil {
		t.Errorf("DecodeDialogue = %+v, want nil when FontLoaded is 0", d)
	}
}

func TestDecodeDialogueOpen(t *testing.T) {
	var m Mem
	m[sym.FontLoaded] = 1
	m[sym.TextBoxID] = 0x2A

	d := DecodeDialogue(&m)
	if d == nil {
		t.Fatal("DecodeDialogue = nil, want open text box")
	}
	if d.TextBoxID != 0x2A {
		t.Errorf("TextBoxID = 0x%02X, want 0x2A", d.TextBoxID)
	}
}

func TestControllable(t *testing.T) {
	var m Mem
	m[sym.FontLoaded] = 0
	m[sym.JoyIgnore] = 0
	m[sym.WalkCounter] = 0
	// Map dimensions are 0: the intro is still running (wCurMap and the
	// coordinates are written during new-game init, long before the map loads).
	if Controllable(&m) {
		t.Fatal("Controllable = true, want false when map dimensions are 0")
	}

	m[sym.CurMapWidth] = 4
	if Controllable(&m) {
		t.Fatal("Controllable = true, want false when only one dimension is non-zero")
	}
	m[sym.CurMapHeight] = 4
	if !Controllable(&m) {
		t.Fatal("Controllable = false, want true when all conditions hold")
	}

	m[sym.FontLoaded] = 1
	if Controllable(&m) {
		t.Errorf("Controllable = true, want false when FontLoaded is non-zero")
	}
	m[sym.FontLoaded] = 0

	m[sym.JoyIgnore] = 1
	if Controllable(&m) {
		t.Errorf("Controllable = true, want false when JoyIgnore is non-zero")
	}
	m[sym.JoyIgnore] = 0

	m[sym.WalkCounter] = 1
	if Controllable(&m) {
		t.Errorf("Controllable = true, want false when WalkCounter is non-zero")
	}
	m[sym.WalkCounter] = 0

	// A hole/Fly warp has written wCurMap but not yet loaded the new map.
	m[sym.StatusFlags6] = 1 << 4
	if Controllable(&m) {
		t.Errorf("Controllable = true, want false while a dungeon warp is pending")
	}
}

// startMenuFixture is the START menu shape measured from run-jxh8lk19wv6on
// (triage:067dd95f2f04909a) after a Water Stone evolved EEVEE on Celadon Mart
// 4F: a double-spaced 7-item menu with ITEM selected.
//
//	wMenuCursorLocation = 0xC423 -> wTileMap offset 131 = (11,6)
//	wCurrentMenuItem    = 2 (POKeDEX, POKeMON, ITEM)
//	wMaxMenuItem        = 7
//	wTopMenuItemY       = 2   (the FIRST item, not the selected one)
//	wTopMenuItemX       = 11
//
// The cursor is four rows below wTopMenuItemY because DrawStartMenu sets
// BIT_DOUBLE_SPACED_MENU: PlaceMenuCursor advances two rows per selected item.
func startMenuFixture(cursorTile byte) *Mem {
	m := &Mem{}
	m[sym.FontLoaded] = 1
	m[sym.MaxMenuItem] = 7
	m[sym.CurrentMenuItem] = 2
	m[sym.TopMenuItemY] = 2
	m[sym.TopMenuItemX] = 11
	m[sym.MenuWatchedKeys] = 0xCB
	const offset = 6*20 + 11
	cursor := sym.TileMap + offset
	m[sym.MenuCursorLocation] = byte(cursor)
	m[sym.MenuCursorLocation+1] = byte(cursor >> 8)
	m[sym.TileMap+offset] = cursorTile
	return m
}

// TestMenuUpReadsMenuCursorLocation is the regression for the stone-use
// deadlock. MenuUp used to read wTopMenuItemX/wTopMenuItemY, which are only the
// FIRST item's coordinates, so a live START menu whose cursor sits on ITEM
// reported "no menu". UseEvolutionItem then could not see the leftover menu it
// had to dismiss, every stone use left that menu open, and the objective died
// on objective_boundary_dirty 318 times in a row on run-jxh8lk19wv6on.
func TestMenuUpReadsMenuCursorLocation(t *testing.T) {
	if !MenuUp(startMenuFixture(menuCursorTile)) {
		t.Fatal("live START menu with the cursor on ITEM read as no menu")
	}
	// A menu that just took a selection shows the unfilled cursor.
	if !MenuUp(startMenuFixture(menuCursorSelectedTile)) {
		t.Fatal("live START menu showing the unfilled cursor read as no menu")
	}
	// The first item's coordinates must not be what decides this: the same
	// bytes with the glyph only under wTopMenuItem=(2,11) are a stale menu.
	stale := startMenuFixture(menuCursorTile)
	staleCursor := sym.TileMap + 11*20 + 11
	stale[sym.MenuCursorLocation] = byte(staleCursor)
	stale[sym.MenuCursorLocation+1] = byte(staleCursor >> 8)
	stale[sym.TileMap+6*20+11] = 0x7F // box border, as measured
	if MenuUp(stale) {
		t.Fatal("cursor bytes pointing at a non-cursor tile decoded as a live menu")
	}
}

// TestMenuUpRejectsCursorOutsideTilemap guards the address arithmetic: a stale
// or nonsense wMenuCursorLocation must fail closed rather than index elsewhere.
func TestMenuUpRejectsCursorOutsideTilemap(t *testing.T) {
	for _, cursor := range []uint16{0x0000, 0xC000, sym.TileMap + sym.TileMapLen} {
		m := startMenuFixture(menuCursorTile)
		m[sym.MenuCursorLocation] = byte(cursor)
		m[sym.MenuCursorLocation+1] = byte(cursor >> 8)
		if MenuUp(m) {
			t.Fatalf("wMenuCursorLocation %#06x outside wTileMap decoded as a live menu", cursor)
		}
	}
}

// A lost battle's whiteout leaves the joypad, font and walk counter idle
// while wIsInBattle=$ff and BIT_BATTLE_OVER_OR_BLACKOUT are still set; the
// respawn has not happened, so the overworld is not controllable yet.
// Values measured on run-22ahrk9pflcilu3jxq9xt37x6's post-Lance checkpoint.
func TestControllableFalseDuringBlackout(t *testing.T) {
	var m Mem
	m[sym.CurMapWidth], m[sym.CurMapHeight] = 13, 13
	m[sym.IsInBattle] = 0xff
	m[sym.StatusFlags4] = 0x2f
	if Controllable(&m) {
		t.Fatal("Controllable = true mid-blackout (wIsInBattle=$ff)")
	}
	m[sym.IsInBattle] = 0
	if Controllable(&m) {
		t.Fatal("Controllable = true with BIT_BATTLE_OVER_OR_BLACKOUT set")
	}
	m[sym.StatusFlags4] = 0x0f
	if !Controllable(&m) {
		t.Fatal("Controllable = false after the respawn cleared both flags")
	}
}
