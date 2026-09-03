package state

import "github.com/maestroi/pokepilot/red/sym"

// BattleKind classifies the kind of battle in progress.
type BattleKind uint8

const (
	BattleNone    BattleKind = 0
	BattleWild    BattleKind = 1
	BattleTrainer BattleKind = 2
)

// CurrentPPMask is the low six bits of a Gen 1 PP byte. The high two bits
// store how many PP Ups were used, so testing the raw byte for non-zero can
// falsely report a move with zero current PP as usable.
const CurrentPPMask uint8 = 0x3f

// Move is one battle move slot.
type Move struct {
	ID uint8 // 0 means the slot is empty
	PP uint8 // current PP; PP Up count bits are stripped during decode
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

	// DisabledMove is the game's 1-based move slot from the high nibble of
	// wPlayerDisabledMove. Zero means no move is disabled. Keeping the RAM's
	// 1..4 encoding makes the zero value of BattleState mean "none disabled"
	// and keeps hand-built test states backwards-compatible.
	DisabledMove uint8

	// Stat stages, biased the way the game stores them: 7 is neutral, lower
	// is worse for us. An opponent spamming GROWL shows up only here.
	ActiveAttackMod  uint8 // wPlayerMonAttackMod
	ActiveDefenseMod uint8 // wPlayerMonDefenseMod
	EnemyAttackMod   uint8 // wEnemyMonAttackMod
	EnemyDefenseMod  uint8 // wEnemyMonDefenseMod

	// The combatants' types. Gen 1 damage is multiplied by the move's
	// effectiveness against BOTH of the defender's types, so a policy that
	// reads only raw power picks a 40-power Normal move over a 40-power
	// Water one against a Rock/Ground opponent — the first does half damage,
	// the second does quadruple. A single-type mon repeats its type in both
	// bytes, exactly as the game stores it.
	EnemyType1  uint8 // wEnemyMonType1
	EnemyType2  uint8 // wEnemyMonType2
	ActiveType1 uint8 // wBattleMonType1
	ActiveType2 uint8 // wBattleMonType2
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

// Usable returns the indices of move slots with ID != 0 and PP > 0 that are
// not currently disabled, in slot order. DisabledMove uses the game's 1-based
// slot encoding, hence i+1. PP is already decoded to current remaining PP;
// the PP Up count bits never make an exhausted move appear usable.
func (b BattleState) Usable() []int {
	var out []int
	for i, mv := range b.Moves {
		if mv.ID != 0 && mv.PP > 0 && b.DisabledMove != uint8(i+1) {
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
		DisabledMove:     m.U8(sym.PlayerDisabledMove) >> 4,
		ActiveAttackMod:  m.U8(sym.PlayerMonAttackMod),
		ActiveDefenseMod: m.U8(sym.PlayerMonDefenseMod),
		EnemyAttackMod:   m.U8(sym.EnemyMonAttackMod),
		EnemyDefenseMod:  m.U8(sym.EnemyMonDefenseMod),
		EnemyType1:       m.U8(sym.EnemyMonType1),
		EnemyType2:       m.U8(sym.EnemyMonType2),
		ActiveType1:      m.U8(sym.BattleMonType1),
		ActiveType2:      m.U8(sym.BattleMonType2),
	}
	for i := 0; i < len(s.Moves); i++ {
		s.Moves[i].ID = m.U8(sym.BattleMonMoves + uint16(i))
		s.Moves[i].PP = m.U8(sym.BattleMonPP+uint16(i)) & CurrentPPMask
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
