package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/combat"
	"github.com/maestroi/pokepilot/red/rom"
)

var _ game.BattleCombatStrategy = (*Profile)(nil)

func (*Profile) EvaluateCombatMove(
	romData []byte,
	attacker, defender game.BattleCombatant,
	nativeMoveID uint16,
	currentPP uint8,
) (game.BattleMoveEvaluation, game.BattleMoveRole, error) {
	moveID, err := gen1CombatByte("move", nativeMoveID)
	if err != nil {
		return game.BattleMoveEvaluation{}, game.BattleMoveRoleOther, err
	}
	atk, err := gen1Combatant(attacker)
	if err != nil {
		return game.BattleMoveEvaluation{}, game.BattleMoveRoleOther, fmt.Errorf("red profile: combat attacker: %w", err)
	}
	def, err := gen1Combatant(defender)
	if err != nil {
		return game.BattleMoveEvaluation{}, game.BattleMoveRoleOther, fmt.Errorf("red profile: combat defender: %w", err)
	}
	mv, err := rom.LookupMove(romData, moveID)
	if err != nil {
		return game.BattleMoveEvaluation{}, game.BattleMoveRoleOther, err
	}

	role := game.BattleMoveRoleOther
	priority := 0
	switch {
	case mv.Power > 0 || mv.Effect == rom.SpecialDamageEffect || mv.Effect == rom.SuperFangEffect || mv.Effect == rom.OHKOEffect:
		role = game.BattleMoveRoleDirectDamage
	case mv.Effect == rom.LeechSeedEffect:
		role, priority = game.BattleMoveRoleResidualDamage, 36
	case mv.Effect == rom.PoisonEffect:
		role, priority = game.BattleMoveRoleResidualDamage, 30
	case mv.Effect == rom.DefenseDown1Effect:
		role = game.BattleMoveRoleSetup
	}

	eval, err := combat.EvaluateMove(romData, atk, def, mv, currentPP)
	if err != nil {
		return game.BattleMoveEvaluation{}, role, err
	}
	return game.BattleMoveEvaluation{
		MoveID:         eval.MoveID,
		NativeMoveID:   nativeMoveID,
		Physical:       eval.Physical,
		AttackStat:     eval.AttackStat,
		DefenseStat:    eval.DefenseStat,
		NeutralDamage:  eval.NeutralDamage,
		Effectiveness:  eval.Effectiveness,
		STAB:           eval.STAB,
		Accuracy:       eval.Accuracy,
		CurrentPP:      eval.CurrentPP,
		MaxPP:          eval.MaxPP,
		DamageRule:     eval.DamageRule,
		ExpectedScore:  eval.ExpectedScore,
		PolicyPriority: priority,
		DebugText:      eval.String(),
	}, role, nil
}

func (*Profile) IncomingTypeRisk(romData []byte, enemy, candidate game.BattleCombatant) int {
	enemyType1, err1 := gen1CombatByte("enemy type 1", enemy.Type1)
	enemyType2, err2 := gen1CombatByte("enemy type 2", enemy.Type2)
	candidateType1, err3 := gen1CombatByte("candidate type 1", candidate.Type1)
	candidateType2, err4 := gen1CombatByte("candidate type 2", candidate.Type2)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return rom.NeutralEffect
	}

	risk := rom.NeutralEffect
	if v, err := rom.TypeEffectiveness(romData, enemyType1, candidateType1, candidateType2); err == nil {
		risk = v
	}
	if enemyType2 != enemyType1 {
		if v, err := rom.TypeEffectiveness(romData, enemyType2, candidateType1, candidateType2); err == nil && v > risk {
			risk = v
		}
	}
	return risk
}

// PreferSetupMove preserves the bounded Red opening-rival behavior without
// leaking Gen-I stat-stage/critical-hit policy into reusable skill code.
func (*Profile) PreferSetupMove(b game.BattleState, _, _ game.BattleMoveEvaluation) bool {
	behind := b.OffenceStage() < 0
	healthy := b.ActiveMaxHP == 0 || b.ActiveHP*2 > b.ActiveMaxHP
	return behind && healthy
}

func (*Profile) IsFieldMove(nativeMoveID uint16) bool {
	switch nativeMoveID {
	case 0x0f, // CUT
		0x13, // FLY
		0x39, // SURF
		0x46, // STRENGTH
		0x94: // FLASH
		return true
	default:
		return false
	}
}

func gen1Combatant(c game.BattleCombatant) (combat.Combatant, error) {
	type1, err := gen1CombatByte("type 1", c.Type1)
	if err != nil {
		return combat.Combatant{}, err
	}
	type2, err := gen1CombatByte("type 2", c.Type2)
	if err != nil {
		return combat.Combatant{}, err
	}
	return combat.Combatant{
		Level:   c.Level,
		HP:      c.HP,
		MaxHP:   c.MaxHP,
		Attack:  c.Attack,
		Defense: c.Defense,
		Special: c.Special,
		Type1:   type1,
		Type2:   type2,
	}, nil
}

func gen1CombatByte(label string, value uint16) (uint8, error) {
	if value > 0xff {
		return 0, fmt.Errorf("%s id %#x exceeds Gen-I byte range", label, value)
	}
	return uint8(value), nil
}
