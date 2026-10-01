package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

var _ game.BattleCombatStrategy = (*Profile)(nil)

func gsLookupMove(nativeMoveID uint16) (power, moveType, accuracy uint8, ok bool) {
	if nativeMoveID == 0 || int(nativeMoveID) >= len(gsMoveMeta) {
		return 0, 0, 0, false
	}
	meta := gsMoveMeta[nativeMoveID]
	return meta[0], meta[1], meta[2], true
}

func (*Profile) EvaluateCombatMove(
	_ []byte,
	attacker, defender game.BattleCombatant,
	nativeMoveID uint16,
	currentPP uint8,
) (game.BattleMoveEvaluation, game.BattleMoveRole, error) {
	power, moveType, accuracy, ok := gsLookupMove(nativeMoveID)
	if !ok {
		return game.BattleMoveEvaluation{NativeMoveID: nativeMoveID, CurrentPP: currentPP}, game.BattleMoveRoleOther, nil
	}
	eff := gsTypeEffectiveness(moveType, uint8(defender.Type1), uint8(defender.Type2))
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
		return eval, game.BattleMoveRoleOther, nil
	}
	return eval, game.BattleMoveRoleDirectDamage, nil
}

func (*Profile) IncomingTypeRisk(_ []byte, enemy, candidate game.BattleCombatant) int {
	risk := gsTypeEffectiveness(uint8(enemy.Type1), uint8(candidate.Type1), uint8(candidate.Type2))
	if enemy.Type2 != enemy.Type1 {
		if v := gsTypeEffectiveness(uint8(enemy.Type2), uint8(candidate.Type1), uint8(candidate.Type2)); v > risk {
			risk = v
		}
	}
	return risk
}

func (*Profile) PreferSetupMove(game.BattleState, game.BattleMoveEvaluation, game.BattleMoveEvaluation) bool {
	return false
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
