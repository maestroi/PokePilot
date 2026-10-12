package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

// A Gen-II battle party pick opens SWITCH/STATS/CANCEL; only the default
// SWITCH row is the switch confirmation the generic transaction answers.
func TestDecodeGoldBattleMonMenuIsSwitchConfirmation(t *testing.T) {
	var mem fakeMemory
	putGSText(&mem, "TYPHLOSION 40 MILTANK 13 CANCEL SWITCH STATS Choose a POKEMON. CANCEL")
	mem[sym.BattleMode] = 1
	mem[sym.TwoDMenuNumRows], mem[sym.TwoDMenuNumCols] = 3, 1
	mem[sym.MenuCursorY], mem[sym.MenuCursorX] = 1, 1
	if got := NewGold().DecodeBattleExecution(&mem).Phase; got != game.BattleExecutionSwitchBox {
		t.Fatalf("phase on SWITCH = %q, want switch box", got)
	}
	mem[sym.MenuCursorY] = 2 // STATS: pressing A would open the stats screen
	if got := NewGold().DecodeBattleExecution(&mem).Phase; got == game.BattleExecutionSwitchBox {
		t.Fatal("STATS row decoded as the switch confirmation")
	}
}
