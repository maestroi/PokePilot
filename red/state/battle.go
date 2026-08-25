package state

import "github.com/maestroi/pokepilot/red/sym"

// BattleKind classifies the kind of battle in progress.
type BattleKind uint8

const (
	BattleNone    BattleKind = 0
	BattleWild    BattleKind = 1
	BattleTrainer BattleKind = 2
)

// Move is one battle move slot.
type Move struct {
	ID uint8 // 0 means the slot is empty
	PP uint8
}

// BattleState is the decoded battle context.
type BattleState struct {
	Kind          BattleKind
	EnemySpecies  uint8
	EnemyHP       uint16
	EnemyMaxHP    uint16
	EnemyLevel    uint8
	ActiveSpecies uint8
	ActiveHP      uint16 // the player's active mon
	ActiveLevel   uint8
	ActiveMaxHP   uint16
	Moves         [4]Move

	// Stat stages, biased the way the game stores them: 7 is neutral, lower
	// is worse for us. An opponent spamming GROWL shows up only here.
	ActiveAttackMod  uint8 // wPlayerMonAttackMod
	ActiveDefenseMod uint8 // wPlayerMonDefenseMod
	EnemyAttackMod   uint8 // wEnemyMonAttackMod
	EnemyDefenseMod  uint8 // wEnemyMonDefenseMod
}

// StatStageNeutral is the value both stat mods hold when nothing has raised
// or lowered them.
const StatStageNeutral uint8 = 7

// OffenceStage reports how much better or worse our physical damage is than
// at the start of the battle: our Attack stage minus the enemy's Defense
// stage. Negative means we are being ground down. Lowering the enemy's
// Defense by a stage cancels a stage lost from our Attack exactly, because
// Gen 1 damage scales on the ratio of the two.
func (b BattleState) OffenceStage() int {
	return int(b.ActiveAttackMod) - int(b.EnemyDefenseMod)
}

// DefenceStage is the mirror of OffenceStage, for the damage coming at us:
// the enemy's Attack stage minus our Defense stage. Positive means they are
// hitting harder than they did at the start.
func (b BattleState) DefenceStage() int {
	return int(b.EnemyAttackMod) - int(b.ActiveDefenseMod)
}

// Usable returns the indices of move slots with ID != 0 and PP > 0, in
// slot order. This is what a move policy picks from.
func (b BattleState) Usable() []int {
	var out []int
	for i, mv := range b.Moves {
		if mv.ID != 0 && mv.PP > 0 {
			out = append(out, i)
		}
	}
	return out
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
	s := &BattleState{
		Kind:             kind,
		EnemySpecies:     m.U8(sym.EnemyMonSpecies),
		EnemyHP:          m.U16BE(sym.EnemyMonHP),
		EnemyMaxHP:       m.U16BE(sym.EnemyMonMaxHP),
		EnemyLevel:       m.U8(sym.EnemyMonLevel),
		ActiveSpecies:    m.U8(sym.BattleMonSpecies),
		ActiveHP:         m.U16BE(sym.BattleMonHP),
		ActiveLevel:      m.U8(sym.BattleMonLevel),
		ActiveMaxHP:      m.U16BE(sym.BattleMonMaxHP),
		ActiveAttackMod:  m.U8(sym.PlayerMonAttackMod),
		ActiveDefenseMod: m.U8(sym.PlayerMonDefenseMod),
		EnemyAttackMod:   m.U8(sym.EnemyMonAttackMod),
		EnemyDefenseMod:  m.U8(sym.EnemyMonDefenseMod),
	}
	for i := 0; i < len(s.Moves); i++ {
		s.Moves[i].ID = m.U8(sym.BattleMonMoves + uint16(i))
		s.Moves[i].PP = m.U8(sym.BattleMonPP + uint16(i))
	}
	return s
}

// BattleResult reports how a finished battle ended.
type BattleResult uint8

const (
	ResultWon BattleResult = iota
	ResultLost
	ResultDraw
)

// DecodeBattleResult decodes wBattleResult.
func DecodeBattleResult(m *Mem) BattleResult {
	return BattleResult(m.U8(sym.BattleResult))
}
