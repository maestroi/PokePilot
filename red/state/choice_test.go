package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

// The fixtures are real captures measured on the ROM 2026-08-28, recorded
// in the comment block in choice.go, not hypotheticals. The predicate is
// pure, so none of these tests needs POKEMON_RED_ROM or an emulator.

// twoOptionFixture builds a Mem with the given menu-shape bytes and, when
// (y,x) is inside the 18x20 screen, the given tile at that tilemap
// position.
func twoOptionFixture(fontLoaded, inBattle, max, cur, y, x, tile byte) *Mem {
	m := &Mem{}
	m[sym.FontLoaded] = fontLoaded
	m[sym.IsInBattle] = inBattle
	m[sym.MaxMenuItem] = max
	m[sym.CurrentMenuItem] = cur
	m[sym.TopMenuItemY] = y
	m[sym.TopMenuItemX] = x
	if int(y) < 18 && int(x) < 20 {
		m[sym.TileMap+uint16(int(y)*20+int(x))] = tile
	}
	return m
}

// TestDecodeTwoOptionMenuLiveCursor is the nurse's live prompt, measured:
// FontLoaded=1, wMaxMenuItem=1, wTopMenuItem=(8,12), tile there 0xED,
// cursor on the first option.
func TestDecodeTwoOptionMenuLiveCursor(t *testing.T) {
	got := DecodeTwoOptionMenu(twoOptionFixture(1, 0, 1, 0, 8, 12, 0xED))
	if got == nil {
		t.Fatal("live prompt with drawn cursor must decode, got nil")
	}
	if got.Index != 0 {
		t.Fatalf("cursor on first option: Index = %d, want 0", got.Index)
	}
}

// TestDecodeTwoOptionMenuSelectedIndex is the same live prompt with
// wCurrentMenuItem=1: the second option is selected.
func TestDecodeTwoOptionMenuSelectedIndex(t *testing.T) {
	got := DecodeTwoOptionMenu(twoOptionFixture(1, 0, 1, 1, 8, 12, 0xED))
	if got == nil {
		t.Fatal("live prompt with drawn cursor must decode, got nil")
	}
	if got.Index != 1 {
		t.Fatalf("cursor on second option: Index = %d, want 1", got.Index)
	}
}

// TestDecodeTwoOptionMenuStaleMenuShape is the Fact 1 regression and the
// case that separates this predicate from yesNoMenuUp: the exact state the
// old predicate returned TRUE on — plain heal prose on screen, stale
// wMaxMenuItem=1, stale wTopMenuItem=(8,12), and the tile there is 0x01,
// not a cursor. It must be nil.
func TestDecodeTwoOptionMenuStaleMenuShape(t *testing.T) {
	if got := DecodeTwoOptionMenu(twoOptionFixture(1, 0, 1, 0, 8, 12, 0x01)); got != nil {
		t.Fatalf("stale menu shape over plain prose must not decode, got %+v", got)
	}
}

// TestDecodeTwoOptionMenuStaleStartMenu is the plain overworld with the
// stale START menu values measured: wMaxMenuItem=3, wTopMenuItem=(12,5),
// tile there 0x0b.
func TestDecodeTwoOptionMenuStaleStartMenu(t *testing.T) {
	if got := DecodeTwoOptionMenu(twoOptionFixture(1, 0, 3, 0, 12, 5, 0x0B)); got != nil {
		t.Fatalf("stale start-menu shape must not decode, got %+v", got)
	}
}

// TestDecodeTwoOptionMenuFontNotLoaded: an otherwise perfect shape with the
// font unloaded and no battle in progress is not a live prompt.
func TestDecodeTwoOptionMenuFontNotLoaded(t *testing.T) {
	if got := DecodeTwoOptionMenu(twoOptionFixture(0, 0, 1, 0, 8, 12, 0xED)); got != nil {
		t.Fatalf("font not loaded: must not decode, got %+v", got)
	}
}

// TestDecodeTwoOptionMenuInBattle is the wild-battle "Use next #MON?" shape:
// wFontLoaded stays 0 for an entire battle (SLICE3-PLAN Addendum 2), so the
// prompt decodes on wIsInBattle with the cursor drawn at the coordinates the
// game stored (hlcoord 13,9 / lb bc, 10, 14 in core.asm DoUseNextMonDialogue
// puts the cursor at row 10, column 14).
func TestDecodeTwoOptionMenuInBattle(t *testing.T) {
	got := DecodeTwoOptionMenu(twoOptionFixture(0, 1, 1, 0, 10, 14, 0xED))
	if got == nil {
		t.Fatal("in-battle prompt with drawn cursor must decode, got nil")
	}
	if got.Index != 0 {
		t.Fatalf("cursor on YES: Index = %d, want 0", got.Index)
	}

	// The staleness guard still fails closed in battle: the same shape with
	// the cursor tile gone (the party menu's ClearScreen has run) is not a
	// live prompt.
	if got := DecodeTwoOptionMenu(twoOptionFixture(0, 1, 1, 0, 10, 14, 0x00)); got != nil {
		t.Fatalf("stale in-battle shape must not decode, got %+v", got)
	}
}

// TestDecodeTwoOptionMenuOutOfRange: stale coordinates that fall off the
// 18x20 screen must return nil, not panic.
func TestDecodeTwoOptionMenuOutOfRange(t *testing.T) {
	cases := []struct {
		name string
		y, x byte
	}{
		{"y at 18", 18, 0},
		{"y wraps", 255, 0},
		{"x at 20", 0, 20},
		{"x wraps", 0, 255},
		{"both off screen", 18, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecodeTwoOptionMenu(twoOptionFixture(1, 0, 1, 0, tc.y, tc.x, 0xED)); got != nil {
				t.Fatalf("out-of-range (%d,%d) must not decode, got %+v", tc.y, tc.x, got)
			}
		})
	}
}
