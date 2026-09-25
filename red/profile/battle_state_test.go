package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeBattleStateProjectsGen1IntoPortableContract(t *testing.T) {
	var mem fakeMemory
	mem[sym.IsInBattle] = 2
	mem[sym.EnemyMonSpecies] = 0x96
	mem[sym.BattleMonSpecies] = 0x03
	mem[sym.BattleMonLevel] = 42
	mem[sym.EnemyMonLevel] = 40
	mem[sym.BattleMonSpecial] = 0
	mem[sym.BattleMonSpecial+1] = 123
	mem[sym.EnemyMonSpecial] = 0
	mem[sym.EnemyMonSpecial+1] = 99
	mem[sym.BattleMonMoves] = 33
	mem[sym.BattleMonMoves+1] = 55
	mem[sym.BattleMonPP] = 0xc5 // PP-Up bits + 5 current PP
	mem[sym.BattleMonPP+1] = 7
	mem[sym.PlayerDisabledMove] = 0x20 // second slot disabled

	b, ok := New().DecodeBattleState(&mem)
	if !ok {
		t.Fatal("trainer battle not decoded")
	}
	if b.Kind != game.BattleTrainer || b.ActiveSpecies != 0x03 || b.EnemySpecies != 0x96 {
		t.Fatalf("identity=%#v", b)
	}
	if b.Moves[0].PP != 5 || b.Moves[0].Disabled {
		t.Fatalf("move 0=%#v want 5 PP and usable", b.Moves[0])
	}
	if !b.Moves[1].Disabled {
		t.Fatalf("disabled state=%#v want disabled", b.Moves[1])
	}
	if b.ActiveSpecialAttack != 123 || b.ActiveSpecialDefense != 123 ||
		b.EnemySpecialAttack != 99 || b.EnemySpecialDefense != 99 {
		t.Fatalf("Gen-I special projection active=%d/%d enemy=%d/%d",
			b.ActiveSpecialAttack, b.ActiveSpecialDefense, b.EnemySpecialAttack, b.EnemySpecialDefense)
	}
}
