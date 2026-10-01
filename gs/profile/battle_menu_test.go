package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func gsTextTile(r rune) byte {
	switch {
	case r >= 'A' && r <= 'Z':
		return 0x80 + byte(r-'A')
	case r >= 'a' && r <= 'z':
		return 0xa0 + byte(r-'a')
	case r >= '0' && r <= '9':
		return 0xf6 + byte(r-'0')
	}
	switch r {
	case ' ':
		return 0x7f
	case '-':
		return 0xe3
	case '?':
		return 0xe6
	case '!':
		return 0xe7
	case '.':
		return 0xe8
	case '/':
		return 0xf3
	case ',':
		return 0xf4
	default:
		return 0x7f
	}
}

func putGSText(mem *fakeMemory, text string) {
	for i := 0; i < sym.TileMapLen; i++ {
		mem[sym.TileMap+uint16(i)] = 0x7f
	}
	for i, r := range text {
		if i >= sym.TileMapLen {
			break
		}
		mem[sym.TileMap+uint16(i)] = gsTextTile(r)
	}
}

func TestDecodeGoldBattleMainMenu(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 2
	mem[sym.TwoDMenuNumRows] = 2
	mem[sym.TwoDMenuNumCols] = 2
	mem[sym.MenuJoypadFilter] = gen2PadA
	mem[sym.MenuCursorY] = 1
	mem[sym.MenuCursorX] = 1
	putGSText(&mem, "FIGHT PKMN PACK RUN")

	menu := NewGold().DecodeBattleMainMenu(&mem)
	if !menu.Visible || menu.Cursor != (game.BattleMenuPosition{Column: 0, Row: 0}) {
		t.Fatalf("main menu=%+v", menu)
	}
	for entry, want := range map[game.BattleMenuEntry]game.BattleMenuPosition{
		game.BattleMenuFight:   {Column: 0, Row: 0},
		game.BattleMenuPokemon: {Column: 1, Row: 0},
		game.BattleMenuItems:   {Column: 0, Row: 1},
		game.BattleMenuRun:     {Column: 1, Row: 1},
	} {
		got, ok := NewGold().BattleMainMenuEntryPosition(entry)
		if !ok || got != want {
			t.Fatalf("%s position=%+v,%v want=%+v,true", entry, got, ok, want)
		}
	}

	// Menu RAM is intentionally persistent after ExitMenu. The rendered FIGHT
	// marker is the positive liveness proof that prevents stale 2x2 state from
	// being treated as a live command surface.
	putGSText(&mem, "CYNDAQUIL used TACKLE!")
	if got := NewGold().DecodeBattleMainMenu(&mem); got.Visible {
		t.Fatalf("stale main menu reported visible: %+v", got)
	}
}

func TestDecodeGoldBattleMoveMenu(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 2
	mem[sym.MoveSelectionMenuType] = 0
	mem[sym.TwoDMenuNumRows] = 4
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2BattleMoveFilter
	mem[sym.MenuCursorY] = 2
	mem[sym.MenuCursorX] = 1
	mem[sym.NumMoves] = 3
	putGSText(&mem, "TACKLE LEER EMBER TYPE/NORMAL")

	cursor := NewSilver().DecodeMenuCursor(&mem)
	if cursor.Current != 2 || cursor.Max != 4 {
		t.Fatalf("move cursor=%+v, want native 1-based {2 4}", cursor)
	}
	exec := NewSilver().DecodeBattleExecution(&mem)
	if exec.Phase != game.BattleExecutionMoveMenu {
		t.Fatalf("execution phase=%q, want move_menu; text=%q", exec.Phase, gsScreenText(&mem))
	}
}

func TestDecodeGoldBattleTwoOption(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 2
	mem[sym.TwoDMenuNumRows] = 2
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	mem[sym.MenuCursorY] = 2
	mem[sym.MenuCursorX] = 1

	prompt, ok := NewGold().DecodeTwoOption(&mem)
	if !ok || prompt.Current != 1 {
		t.Fatalf("two-option=%+v,%v want NO/1", prompt, ok)
	}
	cursor := NewGold().DecodeMenuCursor(&mem)
	if cursor.Current != 1 || cursor.Max != 1 {
		t.Fatalf("normalized prompt cursor=%+v", cursor)
	}

	putGSText(&mem, "FALKNER is about to use PIDGEOTTO. Will GOLD change POKEMON?")
	exec := NewGold().DecodeBattleExecution(&mem)
	if exec.Phase != game.BattleExecutionTrainerSwitch {
		t.Fatalf("trainer switch phase=%q text=%q", exec.Phase, gsScreenText(&mem))
	}

	putGSText(&mem, "Use next POKEMON?")
	exec = NewGold().DecodeBattleExecution(&mem)
	if exec.Phase != game.BattleExecutionUseNextPrompt {
		t.Fatalf("use-next phase=%q text=%q", exec.Phase, gsScreenText(&mem))
	}
}

func TestDecodeGoldBattleMoveLearnerProjection(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 1
	mem[sym.PartyCount] = 2
	mem[sym.CurPartyMon] = 1
	mem[sym.CurBattleMon] = 1
	mem[sym.PutativeTMHMMove] = 79 // PoisonPowder
	mem[sym.BattleMonType1] = 0x16
	mem[sym.BattleMonType2] = 0x16

	base := sym.PartyMon1 + sym.PartyMonSize
	mem[base+gen2PartyMovesOffset+0] = 33
	mem[base+gen2PartyMovesOffset+1] = 45
	mem[base+gen2PartyMovesOffset+2] = 75
	mem[base+gen2PartyMovesOffset+3] = 115

	putGSText(&mem, "CHIKORITA is trying to learn POISONPOWDER!")
	exec := NewGold().DecodeBattleExecution(&mem)
	if exec.Phase != game.BattleExecutionTryLearnPrompt {
		t.Fatalf("phase=%q want try_learn_prompt; text=%q", exec.Phase, gsScreenText(&mem))
	}
	if exec.OfferedMove != 79 {
		t.Fatalf("OfferedMove=%d want 79", exec.OfferedMove)
	}
	if !exec.Learner.Valid || exec.Learner.PartySlot != 1 {
		t.Fatalf("learner=%+v want valid slot 1", exec.Learner)
	}
	if exec.Learner.Moves != [4]uint16{33, 45, 75, 115} {
		t.Fatalf("learner moves=%v", exec.Learner.Moves)
	}
	if exec.Learner.Type1 != 0x16 || exec.Learner.Type2 != 0x16 {
		t.Fatalf("learner types=(%#02x,%#02x) want battle-mon grass", exec.Learner.Type1, exec.Learner.Type2)
	}

	// A non-active learner still projects moves/offered move, but types stay
	// unknown because party structs do not carry them.
	mem[sym.CurBattleMon] = 0
	exec = NewGold().DecodeBattleExecution(&mem)
	if !exec.Learner.Valid || exec.Learner.PartySlot != 1 || exec.Learner.Type1 != 0 || exec.Learner.Type2 != 0 {
		t.Fatalf("non-active learner=%+v", exec.Learner)
	}
}

func TestDecodeGoldBattleRuntime(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 1
	mem[sym.MapGroup] = 24
	mem[sym.MapNumber] = 4
	mem[sym.XCoord] = 7
	mem[sym.YCoord] = 8
	putGSText(&mem, "A wild PIDGEY appeared!")

	live := NewGold().DecodeBattleRuntime(&mem)
	if !live.InBattle || live.NativeMapID != 0x1804 || live.X != 7 || live.Y != 8 {
		t.Fatalf("runtime=%+v", live)
	}
	if live.DebugText == "" {
		t.Fatal("battle runtime omitted rendered debug text")
	}
}

// The Route 29 Dude's catching demo draws a live-looking command menu that
// the ROM steers itself; it must never be reported as a player decision
// (run-1lf849uc4815y2tkvu2odh07vc).
func TestDecodeGoldBattleMainMenuIgnoresTutorialBattle(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 1
	mem[sym.TwoDMenuNumRows] = 2
	mem[sym.TwoDMenuNumCols] = 2
	mem[sym.MenuJoypadFilter] = gen2PadA
	mem[sym.MenuCursorY] = 1
	mem[sym.MenuCursorX] = 1
	putGSText(&mem, "FIGHT PKMN PACK RUN")
	if !NewGold().DecodeBattleMainMenu(&mem).Visible {
		t.Fatal("ordinary battle menu should be visible")
	}
	mem[sym.BattleType] = gen2BattleTypeTutorial
	if got := NewGold().DecodeBattleMainMenu(&mem); got.Visible {
		t.Fatalf("tutorial battle menu reported visible: %+v", got)
	}
}
