package state

import "github.com/maestroi/pokepilot/red/sym"

// BattleKind classifies the kind of battle in progress.
type BattleKind uint8

const (
	BattleNone    BattleKind = 0
	BattleWild    BattleKind = 1
	BattleTrainer BattleKind = 2
)

// BattleState is the decoded battle context.
type BattleState struct {
	Kind         BattleKind
	EnemySpecies uint8
	EnemyHP      uint16
	EnemyMaxHP   uint16
	ActiveHP     uint16 // the player's active mon
	ActiveLevel  uint8
}

// DecodeBattle returns nil when no battle is in progress.
func DecodeBattle(m *Mem) *BattleState {
	var kind BattleKind
	switch m.U8(sym.IsInBattle) {
	case 1:
		kind = BattleWild
	case 2:
		kind = BattleTrainer
	default:
		return nil
	}
	return &BattleState{
		Kind:         kind,
		EnemySpecies: m.U8(sym.EnemyMonSpecies),
		EnemyHP:      m.U16BE(sym.EnemyMonHP),
		// EnemyMaxHP sits two bytes after the EnemyHP pair. If this ever
		// looks wrong, confirm it against pokered.sym's wEnemyMonMaxHP
		// label in a later task.
		EnemyMaxHP:  m.U16BE(sym.EnemyMonHP + 2),
		ActiveHP:    m.U16BE(sym.BattleMonHP),
		ActiveLevel: m.U8(sym.BattleMonLevel),
	}
}
