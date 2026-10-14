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

	// From pokegold/constants/item_data_constants.asm, not item ids.
	gsHeldEffectLeftovers  uint8 = 3
	gsHeldEffectBoostFirst uint8 = 50
)

// TypeBoostItems in pokegold/data/types/type_boost_items.asm maps consecutive
// held-effect ids 50..66 onto these native type ids (including Dark/Steel).
var gsHeldBoostTypes = [...]uint8{
	gsTypeNormal, gsTypeFighting, gsTypeFlying, gsTypePoison,
	gsTypeGround, gsTypeRock, gsTypeBug, gsTypeGhost,
	gsTypeFire, gsTypeWater, gsTypeGrass, gsTypeElectric,
	gsTypePsychic, gsTypeIce, gsTypeDragon, gsTypeDark, gsTypeSteel,
}

// gsHeldEffectModifier uses the cartridge's item attributes, not a second
// hardcoded item-id list. Unsupported effects deliberately remain neutral.
func gsHeldEffectModifier(attr gsrom.ItemAttribute, moveType uint8, maxHP uint16) game.BattleItemModifier {
	if attr.HeldEffect == gsHeldEffectLeftovers && maxHP > 0 {
		// Gen-II Leftovers restores max(1, floor(maxHP/16)) per turn.
		heal := maxHP / 16
		if heal == 0 {
			heal = 1
		}
		return game.BattleItemModifier{RecoveryPerTurn: heal}
	}
	if attr.HeldEffect >= gsHeldEffectBoostFirst {
		index := int(attr.HeldEffect - gsHeldEffectBoostFirst)
		if index < len(gsHeldBoostTypes) && gsHeldBoostTypes[index] == moveType {
			return game.BattleItemModifier{DamageBonusPercent: uint16(attr.Param)}
		}
	}
	return game.BattleItemModifier{}
}

func gsHeldItemModifier(rom []byte, itemID uint16, moveType uint8, maxHP uint16) game.BattleItemModifier {
	if itemID == 0 || itemID > 0xff {
		return game.BattleItemModifier{}
	}
	attr, err := gsrom.LookupItem(rom, uint8(itemID))
	if err != nil {
		return game.BattleItemModifier{}
	}
	return gsHeldEffectModifier(attr, moveType, maxHP)
}

// gsScoreWithItem applies the adapter-projected damage percentage to its
// existing battle score; a zero modifier leaves the score identical.
func gsScoreWithItem(score int64, modifier game.BattleItemModifier) int64 {
	return score + score*int64(modifier.DamageBonusPercent)/100
}

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
	modifier := gsHeldItemModifier(rom, attacker.HeldItem, moveType, attacker.MaxHP)
	score = gsScoreWithItem(score, modifier)
	eval := game.BattleMoveEvaluation{
		MoveID:        uint8(nativeMoveID),
		NativeMoveID:  nativeMoveID,
		Effectiveness: eff,
		STAB:          stab,
		Accuracy:      accuracy,
		CurrentPP:     currentPP,
		ItemModifier:  modifier,
		ExpectedScore: score,
		DebugText:     fmt.Sprintf("move=%d power=%d type=%d eff=%d stab=%v score=%d item=%d boost=%d%% heal=%d", nativeMoveID, power, moveType, eff, stab, score, attacker.HeldItem, modifier.DamageBonusPercent, modifier.RecoveryPerTurn),
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
