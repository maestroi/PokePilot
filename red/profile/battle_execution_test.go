package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/sym"
)

func putTileText(mem *fakeMemory, offset int, text string) {
	for i := 0; i < len(text); i++ {
		c := text[i]
		tile := byte(0x7f)
		switch {
		case c >= 'A' && c <= 'Z':
			tile = 0x80 + c - 'A'
		case c >= 'a' && c <= 'z':
			tile = 0xa0 + c - 'a'
		case c == '!':
			tile = 0xe7
		}
		mem[int(sym.TileMap)+offset+i] = tile
	}
}

// With the cursor on the Disabled move, PrintMenuItem draws "disabled!" at
// hlcoord 1,10 instead of the TYPE/ panel. That is still the move menu
// (run-3udyosuwldeu31xrainttlpo0s stalled on "select FIGHT" without this).
func TestDecodeBattleExecutionMoveMenuWithCursorOnDisabledMove(t *testing.T) {
	var mem fakeMemory
	for i := 0; i < 360; i++ {
		mem[int(sym.TileMap)+i] = 0x7f
	}
	putTileText(&mem, 10*20+1, "disabled!")
	putTileText(&mem, 12*20+6, "CONFUSION")
	if got := New().DecodeBattleExecution(&mem).Phase; got != game.BattleExecutionMoveMenu {
		t.Fatalf("phase=%q want %q", got, game.BattleExecutionMoveMenu)
	}

	// The same word inside the battle message box is not the move panel.
	var msg fakeMemory
	for i := 0; i < 360; i++ {
		msg[int(sym.TileMap)+i] = 0x7f
	}
	putTileText(&msg, 16*20+1, "SWIFT was disabled!")
	if got := New().DecodeBattleExecution(&msg).Phase; got == game.BattleExecutionMoveMenu {
		t.Fatalf("battle message decoded as move menu")
	}
}

// Core.asm skips the move menu after FIGHT when the active mon is asleep or
// frozen, or a Bide/Wrap turn is locked in (run-39etso0zuq4451wr2duvk128ph).
func TestDecodeBattleExecutionMoveSelectionSkipped(t *testing.T) {
	cases := []struct {
		name string
		addr uint16
		val  byte
		want bool
	}{
		{"healthy", sym.BattleMonStatus, 0, false},
		{"poisoned", sym.BattleMonStatus, 1 << 3, false},
		{"asleep", sym.BattleMonStatus, 2, true},
		{"frozen", sym.BattleMonStatus, 1 << 5, true},
		{"bide", sym.PlayerBattleStatus1, 1 << 0, true},
		{"player wrap", sym.PlayerBattleStatus1, 1 << 5, true},
		{"enemy wrap", sym.EnemyBattleStatus1, 1 << 5, true},
	}
	for _, c := range cases {
		var mem fakeMemory
		// Every case below is about a status that skips the menu on its own, so
		// give the active mon a selectable move and isolate the status bit.
		putBattlePP(&mem, [4]byte{10, 10, 10, 10})
		mem[int(c.addr)] = c.val
		if got := New().DecodeBattleExecution(&mem).MoveSelectionSkipped; got != c.want {
			t.Errorf("%s: MoveSelectionSkipped=%v want %v", c.name, got, c.want)
		}
	}
}

// MoveSelectionMenu.regularmenu calls AnyMoveToSelect before drawing anything,
// and AnyMoveToSelect selects STRUGGLE instead when no move the cursor can pick
// has PP left. FIGHT then runs the whole turn, so the shared battle loop must
// not wait for a move menu that will never be drawn
// (run-39etso0zuq4451wr2duvk128ph: Oddish out of PP against the Silph rival).
func TestDecodeBattleExecutionMoveSelectionSkippedWithoutPP(t *testing.T) {
	cases := []struct {
		name     string
		pp       [4]byte
		disabled byte // high nibble of wPlayerDisabledMove: 1-based move index
		want     bool
	}{
		{"one move with pp", [4]byte{1, 0, 0, 0}, 0, false},
		{"all moves spent", [4]byte{0, 0, 0, 0}, 0, true},
		{"only the disabled move has pp", [4]byte{0, 5, 0, 0}, 2, true},
		{"a move the cursor can pick has pp", [4]byte{0, 5, 0, 0}, 1, false},
		{"pp up bits do not count as pp", [4]byte{0xc0, 0, 0, 0}, 0, true},
	}
	for _, c := range cases {
		var mem fakeMemory
		putBattlePP(&mem, c.pp)
		mem[int(sym.PlayerDisabledMove)] = c.disabled << 4
		if got := New().DecodeBattleExecution(&mem).MoveSelectionSkipped; got != c.want {
			t.Errorf("%s: MoveSelectionSkipped=%v want %v", c.name, got, c.want)
		}
	}
}

func putBattlePP(mem *fakeMemory, pp [4]byte) {
	for slot, value := range pp {
		mem[int(sym.BattleMonPP)+slot] = value
	}
}

// TestDecodeBattleExecutionIgnoresScriptedOldManMenu pins the Viridian catch
// tutorial shape seen in run-1jc1gst1w5tv2f. The Old Man battle renders the
// ordinary FIGHT label, but its menu registers still belong to the scripted
// item-list sequence (wMaxMenuItem=4). Battle must leave that screen to the ROM
// instead of treating it as a player-owned command menu. A real command menu
// with the same tiles and wMaxMenuItem=1 remains actionable.
func TestDecodeBattleExecutionIgnoresScriptedOldManMenu(t *testing.T) {
	var mem fakeMemory
	for i := 0; i < 360; i++ {
		mem[int(sym.TileMap)+i] = 0x7f
	}
	mem[int(sym.IsInBattle)] = 1
	putTileText(&mem, 14*20+9, battleMainMenuMarker)

	mem[int(sym.MaxMenuItem)] = 4
	if got := New().DecodeBattleExecution(&mem).Phase; got == game.BattleExecutionMainMenu {
		t.Fatalf("scripted Old Man menu decoded as player command menu: phase=%q", got)
	}

	mem[int(sym.MaxMenuItem)] = redBattleMenuCommandMax
	if got := New().DecodeBattleExecution(&mem).Phase; got != game.BattleExecutionMainMenu {
		t.Fatalf("real command menu phase=%q want %q", got, game.BattleExecutionMainMenu)
	}
}
