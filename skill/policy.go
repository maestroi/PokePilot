package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

// statAwareMoveWithStrategy is the reusable move policy. Generation mechanics
// are projected by strategy: ROM lookup, damage scoring, move roles and setup
// semantics never leak into this file.
func statAwareMoveWithStrategy(romData []byte, strategy game.BattleCombatStrategy) MovePolicy {
	if strategy == nil {
		return FirstUsableMove
	}
	return func(b game.BattleState) int {
		usable := b.Usable()
		if len(usable) == 0 {
			return -1
		}

		attacker, defender := battleCombatants(b)
		bestAttack := -1
		var bestEval game.BattleMoveEvaluation
		haveEval := false
		fallbackAttack := -1
		setupMove := -1
		var setupEval game.BattleMoveEvaluation
		residualDamage := -1
		residualQuality := -1
		lowestPP := -1
		lowestPPLeft := int(^uint(0) >> 1)

		for _, i := range usable {
			// If no decoded move can make HP progress, consume the shortest
			// remaining resource first. That reaches the cartridge's legal
			// STRUGGLE fallback sooner instead of burning a long status pool.
			if pp := int(b.Moves[i].PP); pp < lowestPPLeft {
				lowestPP, lowestPPLeft = i, pp
			}

			eval, role, err := strategy.EvaluateCombatMove(
				romData,
				attacker,
				defender,
				uint16(b.Moves[i].ID),
				b.Moves[i].PP,
			)
			if err != nil {
				if fallbackAttack < 0 {
					fallbackAttack = i
				}
				continue
			}
			switch role {
			case game.BattleMoveRoleDirectDamage:
				if !haveEval || game.BetterBattleMove(eval, bestEval) {
					bestAttack, bestEval, haveEval = i, eval, true
				}
			case game.BattleMoveRoleResidualDamage:
				// Residual progress is preferred over inert status turns only
				// when direct damage is unavailable.
				if eval.PolicyPriority > residualQuality {
					residualDamage, residualQuality = i, eval.PolicyPriority
				}
			case game.BattleMoveRoleSetup:
				if setupMove < 0 || eval.PolicyPriority > setupEval.PolicyPriority {
					setupMove, setupEval = i, eval
				}
			}
		}

		if bestAttack >= 0 && setupMove >= 0 && strategy.PreferSetupMove(b, setupEval, bestEval) {
			if zbatDebug {
				fmt.Printf("zbat policy=setup slot=%d reason=generation-strategy\n", setupMove)
			}
			return setupMove
		}
		if bestAttack >= 0 {
			if zbatDebug {
				fmt.Printf("zbat policy=attack slot=%d %s\n", bestAttack, bestEval.String())
			}
			return bestAttack
		}
		if fallbackAttack >= 0 {
			return fallbackAttack
		}
		if residualDamage >= 0 {
			if zbatDebug {
				fmt.Printf("zbat policy=residual slot=%d reason=no-direct-damage\n", residualDamage)
			}
			return residualDamage
		}
		if lowestPP >= 0 {
			if zbatDebug {
				fmt.Printf("zbat policy=exhaust slot=%d pp=%d reason=no-hp-progress-move\n", lowestPP, lowestPPLeft)
			}
			return lowestPP
		}
		return usable[0]
	}
}
