package state

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/sym"
)

// Compatibility aliases keep existing Gen-I strategy/tests source-stable while
// the reusable battle controller moves to game.BattleState.
type BattleKind = game.BattleKind

const (
	BattleNone    = game.BattleNone
	BattleWild    = game.BattleWild
	BattleTrainer = game.BattleTrainer
)

// CurrentPPMask is the low six bits of a Gen 1 PP byte. The high two bits
// store how many PP Ups were used.
const CurrentPPMask uint8 = 0x3f

type Move = game.BattleMove
type BattleState = game.BattleState

const StatStageNeutral = game.StatStageNeutral

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
	disabled := m.U8(sym.PlayerDisabledMove) >> 4
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
		ActiveAttack:     m.U16BE(sym.BattleMonAttack),
		ActiveDefense:    m.U16BE(sym.BattleMonDefense),
		ActiveSpecial:    m.U16BE(sym.BattleMonSpecial),
		EnemyAttack:      m.U16BE(sym.EnemyMonAttack),
		EnemyDefense:     m.U16BE(sym.EnemyMonDefense),
		EnemySpecial:     m.U16BE(sym.EnemyMonSpecial),
		DisabledMove:     disabled,
		ActiveAttackMod:  m.U8(sym.PlayerMonAttackMod),
		ActiveDefenseMod: m.U8(sym.PlayerMonDefenseMod),
		EnemyAttackMod:   m.U8(sym.EnemyMonAttackMod),
		EnemyDefenseMod:  m.U8(sym.EnemyMonDefenseMod),
		EnemyType1:       m.U8(sym.EnemyMonType1),
		EnemyType2:       m.U8(sym.EnemyMonType2),
		ActiveType1:      m.U8(sym.BattleMonType1),
		ActiveType2:      m.U8(sym.BattleMonType2),
	}
	// Gen I has one Special stat; fill both split fields so callers that use the
	// portable Gen-II-shaped names see the same live value.
	s.ActiveSpecialAttack = s.ActiveSpecial
	s.ActiveSpecialDefense = s.ActiveSpecial
	s.EnemySpecialAttack = s.EnemySpecial
	s.EnemySpecialDefense = s.EnemySpecial
	for i := 0; i < len(s.Moves); i++ {
		s.Moves[i].ID = m.U8(sym.BattleMonMoves + uint16(i))
		s.Moves[i].PP = m.U8(sym.BattleMonPP+uint16(i)) & CurrentPPMask
		s.Moves[i].Disabled = disabled == uint8(i+1)
	}
	return s
}

type BattleResult = game.BattleResult

const (
	ResultWon  = game.BattleWon
	ResultLost = game.BattleLost
	ResultDraw = game.BattleDraw
)

// DecodeBattleResult decodes wBattleResult into the portable result enum.
func DecodeBattleResult(m *Mem) BattleResult {
	return BattleResult(m.U8(sym.BattleResult))
}
