package state

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeBattleNone(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 0
	if s := DecodeBattle(&m); s != nil {
		t.Errorf("DecodeBattle = %+v, want nil for IsInBattle 0", s)
	}
}

func TestDecodeBattleWild(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 1
	m[sym.EnemyMonSpecies] = 16
	// Enemy HP is big-endian: 0x0014 = 20.
	m[sym.EnemyMonHP] = 0x00
	m[sym.EnemyMonHP+1] = 0x14
	m[sym.BattleMonHP] = 0x00
	m[sym.BattleMonHP+1] = 0x20
	m[sym.BattleMonLevel] = 15

	s := DecodeBattle(&m)
	if s == nil {
		t.Fatal("DecodeBattle = nil, want wild battle")
	}
	if s.Kind != BattleWild {
		t.Errorf("Kind = %d, want BattleWild", s.Kind)
	}
	if s.EnemySpecies != 16 {
		t.Errorf("EnemySpecies = %d, want 16", s.EnemySpecies)
	}
	if s.EnemyHP != 20 {
		t.Errorf("EnemyHP = %d, want 20 (big-endian 0x0014)", s.EnemyHP)
	}
	if s.ActiveHP != 32 {
		t.Errorf("ActiveHP = %d, want 32", s.ActiveHP)
	}
	if s.ActiveLevel != 15 {
		t.Errorf("ActiveLevel = %d, want 15", s.ActiveLevel)
	}
}

func TestDecodeBattleTrainer(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 2

	s := DecodeBattle(&m)
	if s == nil {
		t.Fatal("DecodeBattle = nil, want trainer battle")
	}
	if s.Kind != BattleTrainer {
		t.Errorf("Kind = %d, want BattleTrainer", s.Kind)
	}
}

func TestDecodeBattleMoves(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 1
	ids := [4]byte{1, 2, 3, 4}
	powers := [4]byte{10, 20, 30, 40}
	for i := 0; i < 4; i++ {
		m[sym.BattleMonMoves+uint16(i)] = ids[i]
		m[sym.BattleMonPP+uint16(i)] = powers[i]
	}

	s := DecodeBattle(&m)
	if s == nil {
		t.Fatal("DecodeBattle = nil, want battle")
	}
	for i := 0; i < 4; i++ {
		if s.Moves[i].ID != ids[i] {
			t.Errorf("Moves[%d].ID = %d, want %d", i, s.Moves[i].ID, ids[i])
		}
		if s.Moves[i].PP != powers[i] {
			t.Errorf("Moves[%d].PP = %d, want %d", i, s.Moves[i].PP, powers[i])
		}
	}
	if got, want := s.Usable(), []int{0, 1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Errorf("Usable() = %v, want %v", got, want)
	}
}

func TestUsableExcludesEmptyAndExhausted(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 1
	// Slot 0 is usable; slot 1 is empty (ID 0) but has stale PP;
	// slot 2 has a move but no PP; slot 3 is usable.
	m[sym.BattleMonMoves] = 1
	m[sym.BattleMonPP] = 5
	m[sym.BattleMonPP+1] = 15
	m[sym.BattleMonMoves+2] = 3
	m[sym.BattleMonPP+3] = 1
	m[sym.BattleMonMoves+3] = 4

	s := DecodeBattle(&m)
	if s == nil {
		t.Fatal("DecodeBattle = nil, want battle")
	}
	if got, want := s.Usable(), []int{0, 3}; !reflect.DeepEqual(got, want) {
		t.Errorf("Usable() = %v, want %v", got, want)
	}
}

func TestDecodeBattleEnemyMaxHPAddress(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 1
	// The stale address (wEnemyMonHP + 2 = 0xCFE8) and the real
	// wEnemyMonMaxHP (0xCFF4) hold different values; the decoder must
	// read the real one.
	m[sym.EnemyMonHP+2] = 0x00
	m[sym.EnemyMonHP+2+1] = 0x11 // 17 at the stale address
	m[sym.EnemyMonMaxHP] = 0x00
	m[sym.EnemyMonMaxHP+1] = 0x32 // 50 at wEnemyMonMaxHP

	s := DecodeBattle(&m)
	if s == nil {
		t.Fatal("DecodeBattle = nil, want battle")
	}
	if s.EnemyMaxHP != 50 {
		t.Errorf("EnemyMaxHP = %d, want 50 (read from wEnemyMonMaxHP 0xCFF4, not 0xCFE8)", s.EnemyMaxHP)
	}
}

func TestDecodeBattleResult(t *testing.T) {
	cases := []struct {
		raw  uint8
		want BattleResult
	}{
		{0, ResultWon},
		{1, ResultLost},
		{2, ResultDraw},
	}
	for _, c := range cases {
		var m Mem
		m[sym.BattleResult] = c.raw
		if got := DecodeBattleResult(&m); got != c.want {
			t.Errorf("DecodeBattleResult = %d for raw %d, want %d", got, c.raw, c.want)
		}
	}
}

func TestDecodeBattleReadsStatStages(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 2
	m[sym.PlayerMonAttackMod] = 5
	m[sym.PlayerMonDefenseMod] = 7
	m[sym.EnemyMonAttackMod] = 7
	m[sym.EnemyMonDefenseMod] = 6

	b := DecodeBattle(&m)
	if b == nil {
		t.Fatal("DecodeBattle returned nil during a trainer battle")
	}
	if b.ActiveAttackMod != 5 || b.EnemyDefenseMod != 6 {
		t.Fatalf("stat mods = atk %d, enemy def %d; want 5 and 6", b.ActiveAttackMod, b.EnemyDefenseMod)
	}
	// Two stages lost from Attack, one taken off their Defense: net -1.
	if got := b.OffenceStage(); got != -1 {
		t.Fatalf("OffenceStage = %d, want -1", got)
	}
	if got := b.DefenceStage(); got != 0 {
		t.Fatalf("DefenceStage = %d, want 0", got)
	}
}

func TestDecodeBattleReadsLiveCombatStats(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 2
	put := func(addr uint16, value uint16) {
		m[addr] = uint8(value >> 8)
		m[addr+1] = uint8(value)
	}
	put(sym.BattleMonAttack, 123)
	put(sym.BattleMonDefense, 87)
	put(sym.BattleMonSpecial, 201)
	put(sym.EnemyMonAttack, 99)
	put(sym.EnemyMonDefense, 144)
	put(sym.EnemyMonSpecial, 77)

	b := DecodeBattle(&m)
	if b == nil {
		t.Fatal("DecodeBattle returned nil during a trainer battle")
	}
	if b.ActiveAttack != 123 || b.ActiveDefense != 87 || b.ActiveSpecial != 201 {
		t.Fatalf("active stats = atk %d def %d special %d; want 123/87/201", b.ActiveAttack, b.ActiveDefense, b.ActiveSpecial)
	}
	if b.EnemyAttack != 99 || b.EnemyDefense != 144 || b.EnemySpecial != 77 {
		t.Fatalf("enemy stats = atk %d def %d special %d; want 99/144/77", b.EnemyAttack, b.EnemyDefense, b.EnemySpecial)
	}
}
