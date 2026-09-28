package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// testMenuReader adapts a decoded RAM snapshot to the reader the profile
// decoders take, so the fixture is pure RAM: no emulator and no ROM.
type testMenuReader struct{ m *state.Mem }

func (r testMenuReader) Peek8(addr uint16) byte { return r.m[addr] }

func (r testMenuReader) PeekInto(addr uint16, dst []byte) { copy(dst, r.m[addr:]) }

// testDrawMarker writes an uppercase marker-shaped string into wTileMap using
// the tile ids gen1.textChars maps back to those letters (0x80+i -> 'A'+i).
func testDrawMarker(mem *state.Mem, offset int, text string) {
	for i := 0; i < len(text); i++ {
		mem[sym.TileMap+uint16(offset+i)] = 0x80 + text[i] - 'A'
	}
}

// escapeMenuRAM draws the command menu the way DisplayBattleMenu leaves it,
// with the cursor glyph published in the arrow column beside `entryX` and
// wTopMenuItemX holding that entry column.
func escapeMenuRAM(entryX byte, maxItem byte) *state.Mem {
	mem := &state.Mem{}
	mem[sym.IsInBattle] = 1
	mem[sym.MaxMenuItem] = maxItem
	mem[sym.CurrentMenuItem] = 0
	mem[sym.TopMenuItemX] = entryX
	mem[sym.TopMenuItemY] = 14
	testDrawMarker(mem, 14*20+9, battleMainMenuMarker)
	testDrawMarker(mem, 16*20+9, "PKMN")
	testDrawMarker(mem, 16*20+15, "RUN")
	// Row 15 is blank in this fixture, so drawing the cursor there cannot erase
	// the FIGHT marker tiles on row 14 and make the text check fail for an
	// unrelated reason. The glyph's tilemap offset is what the decoder reads.
	cursor := uint16(sym.TileMap + 15*20 + uint16(entryX))
	mem[sym.MenuCursorLocation] = byte(cursor)
	mem[sym.MenuCursorLocation+1] = byte(cursor >> 8)
	mem[sym.TileMap+15*20+uint16(entryX)] = 0xED
	return mem
}

// TestDecodeBattleEscapeMenuIgnoresMoveMenuLeftovers is the deterministic
// regression for run-1wsyy1f75ssxsheu3o4xpui4 (triage:cfe63dc059a15aa5).
//
// The command menu's FIGHT/ITEM/RUN tiles stay in the tilemap while a submenu
// is open, so the move menu shows a leftover "FIGHT" on row 14 above its own
// "ITEM RUN" row. Decoding that as the command menu gave Flee a cursor inside
// the move panel (offset 265 = row 13 col 5), which projected to column -1 and
// failed closed with "battle escape cursor stuck at col=-1 row=0, want RUN at
// col=1 row=1".
//
// The command menu is one row of two columns (wMaxMenuItem == 1); the move menu
// reports the number of moves. The leftover must not decode as a RUN menu.
func TestDecodeBattleEscapeMenuIgnoresMoveMenuLeftovers(t *testing.T) {
	// The exact measured state: leftover FIGHT on screen, move menu open with
	// four moves and its cursor at row 13 col 5.
	moveMenu := escapeMenuRAM(5, 4)
	if got := New().DecodeBattleEscapeMenu(testMenuReader{moveMenu}); got.Visible {
		t.Fatalf("move menu decoded as a RUN menu: %+v", got)
	}
}

// TestDecodeBattleEscapeMenuReadsColumnFromDrawnCursor pins the column source.
// wTopMenuItemX is shared with the move menu and other screens, so a leftover
// x=5 must not decide the column while the command menu's cursor is drawn on
// the right-hand column.
func TestDecodeBattleEscapeMenuReadsColumnFromDrawnCursor(t *testing.T) {
	live := escapeMenuRAM(redBattleMenuRightX, redBattleMenuCommandMax)
	live[sym.TopMenuItemX] = 0x05 // stale move-menu x

	got := New().DecodeBattleEscapeMenu(testMenuReader{live})
	if !got.Visible {
		t.Fatal("live command menu not reported visible")
	}
	if got.Kind != game.BattleEscapeMenuOrdinary {
		t.Fatalf("kind = %q, want ordinary", got.Kind)
	}
	if got.Cursor.Column != 1 {
		t.Fatalf("column = %d, want 1 from the drawn cursor (wTopMenuItemX was a stale 5)", got.Cursor.Column)
	}
}

// TestDecodeBattleEscapeMenuReadsLeftColumn keeps the left column working, so
// the fix cannot pass by only ever reporting the right one.
func TestDecodeBattleEscapeMenuReadsLeftColumn(t *testing.T) {
	got := New().DecodeBattleEscapeMenu(testMenuReader{escapeMenuRAM(redBattleMenuLeftX, redBattleMenuCommandMax)})
	if !got.Visible || got.Cursor.Column != 0 {
		t.Fatalf("left column = %+v, want visible column 0", got)
	}
}
