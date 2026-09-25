package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestTradeSelectionMenuDoesNotRequireDecodedScreenText(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = tradeCenterMapID
	mem[sym.PartyCount] = 2
	mem[sym.TopMenuItemY] = 1
	mem[sym.TopMenuItemX] = 1
	mem[sym.CurrentMenuItem] = 0
	mem[sym.MaxMenuItem] = 2

	// Leave the tilemap zeroed. This reproduces the observed failure where
	// ScreenText returned "" while the ROM was already polling the player
	// party menu in the Trade Center.
	if got := state.ScreenText(&mem); got != "" {
		t.Fatalf("ScreenText = %q, want empty regression fixture", got)
	}
	if !tradeSelectionMenu(&mem) {
		t.Fatal("live Trade Center player menu rejected because screen text is empty")
	}
}

func TestTradeSelectionMenuRequiresROMControllerShape(t *testing.T) {
	base := func() state.Mem {
		var mem state.Mem
		mem[sym.CurMap] = tradeCenterMapID
		mem[sym.PartyCount] = 2
		mem[sym.TopMenuItemY] = 1
		mem[sym.TopMenuItemX] = 1
		mem[sym.CurrentMenuItem] = 0
		mem[sym.MaxMenuItem] = 2
		return mem
	}

	tests := []struct {
		name string
		edit func(*state.Mem)
	}{
		{"wrong map", func(mem *state.Mem) { mem[sym.CurMap] = 0 }},
		{"no party", func(mem *state.Mem) { mem[sym.PartyCount] = 0 }},
		{"wrong y", func(mem *state.Mem) { mem[sym.TopMenuItemY] = 9 }},
		{"wrong x", func(mem *state.Mem) { mem[sym.TopMenuItemX] = 11 }},
		{"wrong max", func(mem *state.Mem) { mem[sym.MaxMenuItem] = 1 }},
		{"cursor outside menu", func(mem *state.Mem) { mem[sym.CurrentMenuItem] = 3 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := base()
			tt.edit(&mem)
			if tradeSelectionMenu(&mem) {
				t.Fatalf("tradeSelectionMenu accepted %s", tt.name)
			}
		})
	}
}
