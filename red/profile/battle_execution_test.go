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
