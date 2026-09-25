package renderstate

import (
	"testing"

	redprofile "github.com/maestroi/pokepilot/red/profile"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	protocol "github.com/maestroi/pokepilot/renderstate"
)

func TestSemanticBattleProjectsActorsMovesAndStatus(t *testing.T) {
	var mem state.Mem
	mem[sym.IsInBattle] = 2
	mem[sym.BattleMonSpecies] = 0xB0 // Charmander
	mem[sym.BattleMonLevel] = 12
	mem[sym.BattleMonHP] = 0
	mem[sym.BattleMonHP+1] = 21
	mem[sym.BattleMonMaxHP] = 0
	mem[sym.BattleMonMaxHP+1] = 31
	mem[sym.BattleMonStatus] = 1 << 3 // poison
	mem[sym.EnemyMonSpecies] = 0x24  // Pidgey
	mem[sym.EnemyMonLevel] = 9
	mem[sym.EnemyMonHP] = 0
	mem[sym.EnemyMonHP+1] = 14
	mem[sym.EnemyMonMaxHP] = 0
	mem[sym.EnemyMonMaxHP+1] = 25
	mem[sym.EnemyMonStatus] = 1 << 6 // paralysis
	mem[sym.BattleMonMoves] = 33     // Tackle
	mem[sym.BattleMonMoves+1] = 39   // Tail Whip
	mem[sym.BattleMonPP] = 12
	mem[sym.BattleMonPP+1] = 17
	mem[sym.PlayerDisabledMove] = 0x20 // slot 2 disabled

	got := semanticBattle(nil, &mem)
	if got == nil {
		t.Fatal("semanticBattle = nil")
	}
	if got.Kind != "trainer" || got.Phase != "action" {
		t.Fatalf("battle kind/phase = %q/%q", got.Kind, got.Phase)
	}
	if len(got.Actors) != 2 {
		t.Fatalf("actors = %d, want 2", len(got.Actors))
	}
	player := got.Actors[0]
	enemy := got.Actors[1]
	if player.Name != "Charmander" || player.Appearance != "charmander" || player.Level != 12 || player.HP != 21 || player.MaxHP != 31 || player.Status != "poisoned" {
		t.Fatalf("player actor = %+v", player)
	}
	if enemy.Name != "Pidgey" || enemy.Appearance != "pidgey" || enemy.Level != 9 || enemy.HP != 14 || enemy.MaxHP != 25 || enemy.Status != "paralyzed" {
		t.Fatalf("enemy actor = %+v", enemy)
	}
	if len(got.Moves) != 2 {
		t.Fatalf("moves = %+v", got.Moves)
	}
	if got.Moves[0].ID != "tackle" || got.Moves[0].Name != "Tackle" || got.Moves[0].PP != 12 || got.Moves[0].Disabled {
		t.Fatalf("move 0 = %+v", got.Moves[0])
	}
	if got.Moves[1].ID != "tail-whip" || got.Moves[1].Name != "Tail Whip" || got.Moves[1].PP != 17 || !got.Moves[1].Disabled {
		t.Fatalf("move 1 = %+v", got.Moves[1])
	}
}

func TestSemanticDialogueUsesRenderedTilemapText(t *testing.T) {
	var mem state.Mem
	mem[sym.FontLoaded] = 1
	writeScreenText(&mem, "HELLO!")

	got := semanticDialogue(&mem)
	if got == nil || got.Text != "HELLO!" {
		t.Fatalf("dialogue = %+v", got)
	}
}

func TestSemanticMenuRequiresLiveCursorAndPublishesSelectionShape(t *testing.T) {
	var mem state.Mem
	mem[sym.FontLoaded] = 1
	mem[sym.CurrentMenuItem] = 1
	mem[sym.MaxMenuItem] = 2
	writeScreenText(&mem, "ITEM POKEMON EXIT")
	cursorOffset := 40
	cursorAddr := sym.TileMap + uint16(cursorOffset)
	mem[sym.MenuCursorLocation] = byte(cursorAddr)
	mem[sym.MenuCursorLocation+1] = byte(cursorAddr >> 8)
	mem[sym.TileMap+uint16(cursorOffset)] = 0xED

	got := semanticMenu(&mem)
	if got == nil || got.Cursor == nil || *got.Cursor != 1 {
		t.Fatalf("menu = %+v", got)
	}
	if len(got.Entries) != 3 {
		t.Fatalf("menu entries = %d, want 3", len(got.Entries))
	}
	if got.Title == "" {
		t.Fatal("menu title/screen text is empty")
	}
}

func TestSnapshotBattleDoesNotDependOnOverworldGeometry(t *testing.T) {
	producer := &Producer{profile: redprofile.New()}
	var mem state.Mem
	mem[sym.IsInBattle] = 1
	mem[sym.BattleMonSpecies] = 0xB0
	mem[sym.BattleMonLevel] = 10
	mem[sym.BattleMonHP+1] = 20
	mem[sym.BattleMonMaxHP+1] = 30
	mem[sym.EnemyMonSpecies] = 0x24
	mem[sym.EnemyMonLevel] = 4
	mem[sym.EnemyMonHP+1] = 9
	mem[sym.EnemyMonMaxHP+1] = 12
	// Deliberately leave map dimensions and ROM empty. A battle scene must not
	// fail because the overworld block buffer is unavailable.

	got, err := producer.Snapshot(memorySnapshot{mem: &mem}, protocol.FrameMeta{Frame: 17})
	if err != nil {
		t.Fatal(err)
	}
	if got.Scene != protocol.SceneBattle || got.Battle == nil || !got.HasCapability(protocol.CapabilityBattle) {
		t.Fatalf("battle snapshot = %+v", got)
	}
}

func TestSnapshotDialogueSurvivesUnavailableWorldGeometry(t *testing.T) {
	producer := &Producer{profile: redprofile.New()}
	var mem state.Mem
	mem[sym.FontLoaded] = 1
	writeScreenText(&mem, "WELCOME!")

	got, err := producer.Snapshot(memorySnapshot{mem: &mem}, protocol.FrameMeta{Frame: 18})
	if err != nil {
		t.Fatal(err)
	}
	if got.Scene != protocol.SceneDialogue || got.Dialogue == nil || got.Dialogue.Text != "WELCOME!" {
		t.Fatalf("dialogue snapshot = %+v", got)
	}
}

func writeScreenText(mem *state.Mem, text string) {
	for i := 0; i < sym.TileMapLen; i++ {
		mem[sym.TileMap+uint16(i)] = 0x7f
	}
	for i, r := range text {
		var tile byte
		switch {
		case r >= 'A' && r <= 'Z':
			tile = 0x80 + byte(r-'A')
		case r >= 'a' && r <= 'z':
			tile = 0xA0 + byte(r-'a')
		case r == '!':
			tile = 0xE7
		case r == '?':
			tile = 0xE6
		case r == ' ':
			tile = 0x7f
		default:
			continue
		}
		mem[sym.TileMap+uint16(i)] = tile
	}
}
