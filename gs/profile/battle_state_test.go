package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func putBattleBE16(mem *fakeMemory, addr uint16, value uint16) {
	mem[addr] = byte(value >> 8)
	mem[addr+1] = byte(value)
}

func TestDecodeGoldBattleState(t *testing.T) {
	var mem fakeMemory
	mem[sym.BattleMode] = 2

	mem[sym.BattleMonSpecies] = 0x9b // Cyndaquil
	mem[sym.BattleMonLevel] = 11
	putBattleBE16(&mem, sym.BattleMonHP, 27)
	putBattleBE16(&mem, sym.BattleMonMaxHP, 31)
	putBattleBE16(&mem, sym.BattleMonAttack, 19)
	putBattleBE16(&mem, sym.BattleMonDefense, 17)
	putBattleBE16(&mem, sym.BattleMonSpeed, 24)
	putBattleBE16(&mem, sym.BattleMonSpclAtk, 22)
	putBattleBE16(&mem, sym.BattleMonSpclDef, 18)
	mem[sym.BattleMonType1] = 0x16
	mem[sym.BattleMonType2] = 0x16
	mem[sym.BattleMonMoves+0] = 33
	mem[sym.BattleMonMoves+1] = 43
	mem[sym.BattleMonPP+0] = 0xc0 | 35 // PP Up bits are not current PP.
	mem[sym.BattleMonPP+1] = 30

	mem[sym.EnemyMonSpecies] = 16 // Pidgey
	mem[sym.EnemyMonLevel] = 9
	putBattleBE16(&mem, sym.EnemyMonHP, 18)
	putBattleBE16(&mem, sym.EnemyMonMaxHP, 22)
	putBattleBE16(&mem, sym.EnemyMonAttack, 14)
	putBattleBE16(&mem, sym.EnemyMonDefense, 13)
	putBattleBE16(&mem, sym.EnemyMonSpeed, 17)
	putBattleBE16(&mem, sym.EnemyMonSpclAtk, 11)
	putBattleBE16(&mem, sym.EnemyMonSpclDef, 12)
	mem[sym.EnemyMonType1] = 0x00
	mem[sym.EnemyMonType2] = 0x02

	mem[sym.PlayerStatLevels] = 8
	mem[sym.PlayerStatLevels+1] = 6
	mem[sym.EnemyStatLevels] = 7
	mem[sym.EnemyStatLevels+1] = 9

	state, ok := NewGold().DecodeBattleState(&mem)
	if !ok {
		t.Fatal("trainer battle was not decoded")
	}
	if state.Kind != game.BattleTrainer {
		t.Fatalf("kind=%v, want trainer", state.Kind)
	}
	if state.ActiveSpecies != 0x9b || state.ActiveLevel != 11 || state.ActiveHP != 27 || state.ActiveMaxHP != 31 {
		t.Fatalf("active identity/hp=%+v", state)
	}
	if state.EnemySpecies != 16 || state.EnemyLevel != 9 || state.EnemyHP != 18 || state.EnemyMaxHP != 22 {
		t.Fatalf("enemy identity/hp=%+v", state)
	}
	if state.ActiveAttack != 19 || state.ActiveDefense != 17 || state.ActiveSpeed != 24 ||
		state.ActiveSpecialAttack != 22 || state.ActiveSpecialDefense != 18 {
		t.Fatalf("active stats=%+v", state)
	}
	if state.EnemyAttack != 14 || state.EnemyDefense != 13 || state.EnemySpeed != 17 ||
		state.EnemySpecialAttack != 11 || state.EnemySpecialDefense != 12 {
		t.Fatalf("enemy stats=%+v", state)
	}
	if state.ActiveAttackMod != 8 || state.ActiveDefenseMod != 6 ||
		state.EnemyAttackMod != 7 || state.EnemyDefenseMod != 9 {
		t.Fatalf("stat stages=%+v", state)
	}
	if state.ActiveType1 != 0x16 || state.ActiveType2 != 0x16 ||
		state.EnemyType1 != 0x00 || state.EnemyType2 != 0x02 {
		t.Fatalf("types=%+v", state)
	}
	if state.Moves[0].ID != 33 || state.Moves[0].PP != 35 || state.Moves[1].ID != 43 || state.Moves[1].PP != 30 {
		t.Fatalf("moves=%+v", state.Moves)
	}
	if got := state.Usable(); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("usable=%v, want [0 1]", got)
	}
}

func TestDecodeGoldBattleStateBoundaries(t *testing.T) {
	var mem fakeMemory
	p := NewSilver()
	if _, ok := p.DecodeBattleState(&mem); ok {
		t.Fatal("overworld decoded as battle")
	}

	mem[sym.BattleMode] = 1
	if state, ok := p.DecodeBattleState(&mem); !ok || state.Kind != game.BattleWild {
		t.Fatalf("wild battle=%+v ok=%v", state, ok)
	}

	for raw, want := range map[byte]game.BattleResult{
		0x00: game.BattleWon,
		0x01: game.BattleLost,
		0x02: game.BattleDraw,
		0x80: game.BattleWon, // box-full flag plus WIN
	} {
		mem[sym.BattleResult] = raw
		if got := p.DecodeBattleResult(&mem); got != want {
			t.Fatalf("result raw=%#02x got=%v want=%v", raw, got, want)
		}
	}
}
