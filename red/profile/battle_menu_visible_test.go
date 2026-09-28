package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// drawMarker writes an uppercase marker-shaped string into wTileMap using the
// same tile ids the ROM draws (gen1.textChars maps 0x80+i onto 'A'+i, matching
// pokered/constants/charmap.asm).
func drawMarker(mem *state.Mem, offset int, text string) {
	for i := 0; i < len(text); i++ {
		mem[sym.TileMap+uint16(offset+i)] = 0x80 + text[i] - 'A'
	}
}

// memReader adapts a decoded RAM snapshot to the reader the profile decoders
// take, so the fixture stays a pure RAM fact with no emulator and no ROM.
type memReader struct{ m *state.Mem }

func (r memReader) Peek8(addr uint16) byte { return r.m[addr] }

func (r memReader) PeekInto(addr uint16, dst []byte) { copy(dst, r.m[addr:]) }

// battleMenuRAM builds the command menu exactly as DisplayBattleMenu leaves it:
// FIGHT/ITEM/PKMN/RUN drawn in the tilemap, the cursor address published, and
// the cursor glyph present only when `drawn` says the menu owns input.
func battleMenuRAM(drawn bool) *state.Mem {
	mem := &state.Mem{}
	mem[sym.IsInBattle] = 2 // trainer battle, enough for DecodeBattle
	// Keep the entries on separate tile rows: ScreenText is one flat
	// normalized run, and "FIGHT" on the same row as "ITEM" would make the F's
	// abut into a pathological display run that NormalizeDisplayText collapses.
	drawMarker(mem, 14*20+9, battleMainMenuMarker)
	drawMarker(mem, 15*20+9, "ITEM")
	drawMarker(mem, 16*20+9, "PKMN")
	mem[sym.TopMenuItemX] = redBattleMenuLeftX
	mem[sym.TopMenuItemY] = 14
	mem[sym.CurrentMenuItem] = 0
	mem[sym.MaxMenuItem] = 1
	// The published cursor address sits on a blank cell, as the real arrow
	// column does. Drawing it over the marker's first letter would erase the
	// "FIGHT" tiles themselves and make the text check fail for a different
	// reason than the one under test.
	cursor := uint16(sym.TileMap + 16*20 + 4)
	mem[sym.MenuCursorLocation] = byte(cursor)
	mem[sym.MenuCursorLocation+1] = byte(cursor >> 8)
	if drawn {
		mem[sym.TileMap+16*20+4] = 0xED
	}
	return mem
}

// TestDecodeBattleMainMenuRequiresDrawnCursor is the deterministic regression
// for run-10zwbyvmdyfnp1tji510e0aqus (triage:18ad741080f93b5d). The command
// menu is drawn once and stays in the tilemap for the whole turn, so while the
// enemy's Wrap ticks and the text box owns the screen, ScreenText still reads
// FIGHT with live-looking wTopMenuItemX/wCurrentMenuItem bytes. Reporting that
// as visible made the battle driver press A into a ROM that was not polling and
// then fail `battle menu "fight" did not open within 500 frames`, which dirtied
// the objective boundary and circuit-broke the run.
//
// The cursor glyph is what separates the two states: it is present exactly
// while PlaceMenuCursor has drawn a menu waiting for input, and absent while
// the text box owns the screen.
func TestDecodeBattleMainMenuRequiresDrawnCursor(t *testing.T) {
	if got := New().DecodeBattleMainMenu(memReader{battleMenuRAM(false)}); got.Visible {
		t.Fatalf("stale menu text reported a visible menu: %+v", got)
	}

	live := New().DecodeBattleMainMenu(memReader{battleMenuRAM(true)})
	if !live.Visible {
		t.Fatal("live command menu with a drawn cursor was reported hidden")
	}
	if live.Cursor.Column != 0 || live.Cursor.Row != 0 {
		t.Fatalf("cursor = %+v, want FIGHT at column 0 row 0", live.Cursor)
	}
}

// TestDecodeBattleMainMenuAcceptsBothCursorGlyphs keeps the unfilled arrow that
// PlaceUnfilledArrowMenuCursor draws mid-selection live, so a save state or a
// frame captured between the press and the branch does not read as no menu.
func TestDecodeBattleMainMenuAcceptsBothCursorGlyphs(t *testing.T) {
	for _, glyph := range []byte{0xED, 0xEC} {
		mem := battleMenuRAM(true)
		mem[sym.TileMap+16*20+4] = glyph
		if got := New().DecodeBattleMainMenu(memReader{mem}); !got.Visible {
			t.Fatalf("cursor glyph %#02x was rejected", glyph)
		}
	}
}

// TestMenuCursorDrawnIsTheSharedPositiveCheck pins the helper the decoder now
// depends on, so a change to the cursor rule cannot silently re-open the hole.
func TestMenuCursorDrawnIsTheSharedPositiveCheck(t *testing.T) {
	if !state.MenuCursorDrawn(battleMenuRAM(true)) {
		t.Fatal("drawn cursor not reported")
	}
	if state.MenuCursorDrawn(battleMenuRAM(false)) {
		t.Fatal("absent cursor reported as drawn")
	}
}
