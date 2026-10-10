package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
)

var _ game.BattleCombatStrategy = (*Profile)(nil)

func gsLookupMove(nativeMoveID uint16) (power, moveType, accuracy uint8, ok bool) {
	if nativeMoveID == 0 || int(nativeMoveID) >= len(gsMoveMeta) {
		return 0, 0, 0, false
	}
	meta := gsMoveMeta[nativeMoveID]
	return meta[0], meta[1], meta[2], true
}

const (
	gsMoveGrowl   uint16 = 45
	gsMoveReflect uint16 = 115
)

// gsTypeEffectiveness reads the cartridge's own TypeMatchups table.
func gsTypeEffectiveness(rom []byte, moveType, def1, def2 uint8) (int, error) {
	return gsrom.TypeEffectiveness(rom, moveType, def1, def2)
}

func (*Profile) EvaluateCombatMove(
	rom []byte,
	attacker, defender game.BattleCombatant,
	nativeMoveID uint16,
	currentPP uint8,
) (game.BattleMoveEvaluation, game.BattleMoveRole, error) {
	power, moveType, accuracy, ok := gsLookupMove(nativeMoveID)
	if !ok {
		return game.BattleMoveEvaluation{NativeMoveID: nativeMoveID, CurrentPP: currentPP}, game.BattleMoveRoleOther, nil
	}
	// Status moves never touch the chart (their effectiveness is unused).
	eff := gsrom.TypeNeutralEffect
	if power > 0 {
		var err error
		if eff, err = gsTypeEffectiveness(rom, moveType, uint8(defender.Type1), uint8(defender.Type2)); err != nil {
			return game.BattleMoveEvaluation{NativeMoveID: nativeMoveID, CurrentPP: currentPP}, game.BattleMoveRoleOther, err
		}
	}
	stab := moveType == uint8(attacker.Type1) || moveType == uint8(attacker.Type2)
	score := int64(power) * int64(eff)
	if stab {
		score = score * 3 / 2
	}
	eval := game.BattleMoveEvaluation{
		MoveID:        uint8(nativeMoveID),
		NativeMoveID:  nativeMoveID,
		Effectiveness: eff,
		STAB:          stab,
		Accuracy:      accuracy,
		CurrentPP:     currentPP,
		ExpectedScore: score,
		DebugText:     fmt.Sprintf("move=%d power=%d type=%d eff=%d stab=%v score=%d", nativeMoveID, power, moveType, eff, stab, score),
	}
	if power == 0 {
		switch nativeMoveID {
		case gsMoveReflect:
			eval.PolicyPriority = 40
			return eval, game.BattleMoveRoleSetup, nil
		case gsMoveGrowl:
			eval.PolicyPriority = 30
			return eval, game.BattleMoveRoleSetup, nil
		default:
			return eval, game.BattleMoveRoleOther, nil
		}
	}
	return eval, game.BattleMoveRoleDirectDamage, nil
}

// IncomingTypeRisk is neutral when the chart cannot be read: risk only ranks
// switch candidates, and an unreadable ROM fails louder in EvaluateCombatMove.
func (*Profile) IncomingTypeRisk(rom []byte, enemy, candidate game.BattleCombatant) int {
	risk, err := gsTypeEffectiveness(rom, uint8(enemy.Type1), uint8(candidate.Type1), uint8(candidate.Type2))
	if err != nil {
		return gsrom.TypeNeutralEffect
	}
	if enemy.Type2 != enemy.Type1 {
		if v, err := gsTypeEffectiveness(rom, uint8(enemy.Type2), uint8(candidate.Type1), uint8(candidate.Type2)); err == nil && v > risk {
			risk = v
		}
	}
	return risk
}

// PreferSetupMove spends Reflect/Growl only against threats that outpace a
// healthy lead's neutral Tackle. Weak gym-trainer bugs should be KO'd with
// damage immediately; Scyther-class Fury Cutter snowballs need the screen.
func (*Profile) PreferSetupMove(b game.BattleState, setup, _ game.BattleMoveEvaluation) bool {
	healthy := b.ActiveMaxHP == 0 || b.ActiveHP*2 > b.ActiveMaxHP
	if !healthy || b.EnemyLevel < 16 {
		return false
	}
	switch setup.NativeMoveID {
	case gsMoveReflect:
		return !b.ActiveReflect
	case gsMoveGrowl:
		return b.EnemyAttackMod >= game.StatStageNeutral
	default:
		return false
	}
}

func (*Profile) IsFieldMove(nativeMoveID uint16) bool {
	switch nativeMoveID {
	case 15, // CUT
		19,  // FLY
		57,  // SURF
		70,  // STRENGTH
		127, // WATERFALL
		148, // FLASH
		250: // WHIRLPOOL
		return true
	default:
		return false
	}
}
