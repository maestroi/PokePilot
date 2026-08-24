package state

import (
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
