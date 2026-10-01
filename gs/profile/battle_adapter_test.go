package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func gsTile(c byte) byte {
	switch {
	case c >= 'A' && c <= 'Z':
		return 0x80 + c - 'A'
	case c >= 'a' && c <= 'z':
		return 0xa0 + c - 'a'
	case c >= '0' && c <= '9':
		return 0xf6 + c - '0'
	}
	switch c {
	case ' ':
		return 0x7f
	case '?':
		return 0xe6
	case '!':
		return 0xe7
	case '.':
		return 0xe8
	case '-':
		return 0xe3
	case '/':
		return 0xf3
	case ',':
		return 0xf4
	default:
		return 0x7f
	}
}

func putGSScreenText(mem *fakeMemory, text string) {
	for i := 0; i < sym.TileMapLen; i++ {
		mem[sym.TileMap+uint16(i)] = 0x7f
	}
	for i := 0; i < len(text) && i < sym.TileMapLen; i++ {
		mem[sym.TileMap+uint16(i)] = gsTile(text[i])
	}
}

func TestGoldBattleMainMenuSemantics(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 2
	mem[sym.TwoDMenuNumRows] = 2
	mem[sym.TwoDMenuNumCols] = 2
	mem[sym.MenuJoypadFilter] = gen2PadA
	mem[sym.MenuCursorY] = 1
	mem[sym.MenuCursorX] = 2
	putGSScreenText(&mem, "FIGHT POKEMON PACK RUN")

	p := NewGold()
	menu := p.DecodeBattleMainMenu(&mem)
	if !menu.Visible || menu.Cursor != (game.BattleMenuPosition{Column: 1, Row: 0}) {
		t.Fatalf("main menu = %+v, want visible at pokemon cell", menu)
	}
	if got := p.DecodeMenuCursor(&mem); got.Current != 0 || got.Max != 1 {
		t.Fatalf("generic cursor = %+v, want row 0 max 1", got)
	}
	if pos, ok := p.BattleMainMenuEntryPosition(game.BattleMenuItems); !ok || pos != (game.BattleMenuPosition{Column: 0, Row: 1}) {
		t.Fatalf("items position = %+v,%v", pos, ok)
	}
}

func TestGoldBattleMoveMenuKeepsFourthNativeCursor(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 2
	mem[sym.MoveSelectionMenuType] = 0
	mem[sym.TwoDMenuNumRows] = 4
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2BattleMoveFilter
	mem[sym.MenuCursorY] = 4
	mem[sym.MenuCursorX] = 1
	putGSScreenText(&mem, "TYPE/NORMAL")

	p := NewGold()
	if got := p.DecodeMenuCursor(&mem); got.Current != 4 || got.Max != 4 {
		t.Fatalf("move cursor = %+v, want native 4/4", got)
	}
	if got := p.DecodeBattleExecution(&mem).Phase; got != game.BattleExecutionMoveMenu {
		t.Fatalf("phase = %q, want move menu", got)
	}
}

func TestGoldUseNextPromptIsTwoOption(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 1
	mem[sym.TwoDMenuNumRows] = 2
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	mem[sym.MenuCursorY] = 2
	mem[sym.MenuCursorX] = 1
	// The answerable prompt renders the YES/NO box on the screen.
	putGSScreenText(&mem, "Use next POKEMON? YES NO")

	p := NewGold()
	prompt, ok := p.DecodeTwoOption(&mem)
	if !ok || prompt.Current != 1 {
		t.Fatalf("prompt = %+v,%v, want NO selected", prompt, ok)
	}
	if got := p.DecodeBattleExecution(&mem).Phase; got != game.BattleExecutionUseNextPrompt {
		t.Fatalf("phase = %q, want use-next prompt", got)
	}
}

func TestGoldBattleResourcesProjectPartyWithoutGen1BagIDs(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 2
	mem[sym.PartyCount] = 2
	mem[sym.CurBattleMon] = 0
	mem[sym.BattleMonType1] = 0x16
	mem[sym.BattleMonType2] = 0x16

	base := sym.PartyMon1
	mem[base] = 0x9b
	mem[base+gen2PartyLevelOffset] = 12
	putBattleBE16(&mem, base+gen2PartyHPOffset, 25)
	putBattleBE16(&mem, base+gen2PartyMaxHPOffset, 32)
	mem[base+gen2PartyMovesOffset] = 33
	mem[base+gen2PartyPPOffset] = 0xc0 | 20

	base2 := sym.PartyMon1 + sym.PartyMonSize
	mem[base2] = 16
	mem[base2+gen2PartyLevelOffset] = 9
	putBattleBE16(&mem, base2+gen2PartyHPOffset, 18)
	putBattleBE16(&mem, base2+gen2PartyMaxHPOffset, 22)
	mem[base2+gen2PartyMovesOffset] = 33
	mem[base2+gen2PartyPPOffset] = 10

	got := NewGold().DecodeBattleResources(&mem)
	if !got.InBattle || got.ActiveSlot != 0 || len(got.Party) != 2 {
		t.Fatalf("resources = %+v", got)
	}
	if got.Party[0].NativeSpeciesID != 0x9b || got.Party[0].Moves[0].PP != 20 ||
		got.Party[0].Type1 != 0x16 || got.Party[0].Type2 != 0x16 {
		t.Fatalf("active party projection = %+v", got.Party[0])
	}
	if len(got.Bag) != 0 {
		t.Fatalf("bag = %+v, want disabled until item ids are portable", got.Bag)
	}
	if slot, ok := got.PPRecoverySlot(); !ok || slot != 1 {
		t.Fatalf("PPRecoverySlot = %d,%v, want 1,true", slot, ok)
	}
}

func TestGoldDecodeBattleExecutionMoveSelectionSkippedWithoutPP(t *testing.T) {
	cases := []struct {
		name string
		pp   [4]byte
		want bool
	}{
		{"one move with pp", [4]byte{1, 0, 0, 0}, false},
		{"all moves spent", [4]byte{0, 0, 0, 0}, true},
		{"pp up bits do not count as pp", [4]byte{0xc0, 0, 0, 0}, true},
	}
	for _, c := range cases {
		var mem fakeMemory
		mem[sym.BattleMode] = 2
		for slot, value := range c.pp {
			mem[sym.BattleMonPP+uint16(slot)] = value
		}
		if got := NewGold().DecodeBattleExecution(&mem).MoveSelectionSkipped; got != c.want {
			t.Errorf("%s: MoveSelectionSkipped=%v want %v", c.name, got, c.want)
		}
	}
}

func TestGoldForcedPartyMenuIncludesCancelCursor(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 1
	mem[sym.PartyCount] = 2
	mem[sym.TwoDMenuNumRows] = 3
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	mem[sym.MenuCursorX] = 1
	mem[sym.MenuCursorY] = 3 // CANCEL row
	putBattleBE16(&mem, sym.BattleMonHP, 0)
	// The live screen renders the party list with a CANCEL row; the bottom
	// prompt is "Which  ?" with no species word.
	putGSScreenText(&mem, "SQUIRTLE 25/ 32 12 CANCEL Which ?")

	got := NewGold().DecodePartyMenu(&mem)
	if !got.Visible || got.Kind != game.PartyMenuForcedBattle {
		t.Fatalf("party menu = %+v, want forced battle", got)
	}
	if got.Cursor.Current != 2 || got.Cursor.Max != 1 {
		t.Fatalf("cursor = %+v, want cancel current 2 with party max 1", got.Cursor)
	}
}
