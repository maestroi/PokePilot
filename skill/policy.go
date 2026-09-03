package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/combat"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

// StatAwareMove is the default policy for a real fight. Damaging moves are
// ranked by the shared Gen 1 combat model: current physical/special stats,
// power, STAB, type effectiveness, accuracy and PP. If the opponent has been
// grinding our physical offence down, the policy can still spend one healthy
// turn lowering the opponent's Defense — the bounded setup behavior that
// fixes the opening rival fight.
//
// This closes two important gaps left by the earlier power/type heuristic:
// equal-power moves can be radically different when one uses a weak Attack
// stat and the other a strong Special stat, and a high-power inaccurate move
// is not automatically a better expected turn than a reliable one.
//
// It closes over the ROM because move metadata and the type chart live there,
// while the live combat stats/types/PP come from BattleState. A move id that
// cannot be decoded is treated as a fallback attack rather than silently
// selecting an unrelated slot.
func StatAwareMove(romData []byte) MovePolicy {
	return func(b state.BattleState) int {
		usable := b.Usable()
		if len(usable) == 0 {
			return -1
		}

		attacker, defender := combat.PlayerMatchup(b)
		bestAttack := -1
		var bestEval combat.MoveEvaluation
		haveEval := false
		fallbackAttack := -1
		defenseDown := -1

		for _, i := range usable {
			mv, err := rom.LookupMove(romData, b.Moves[i].ID)
			if err != nil {
				if fallbackAttack < 0 {
					fallbackAttack = i
				}
				continue
			}
			switch {
			case mv.Power > 0:
				eval, err := combat.EvaluateMove(romData, attacker, defender, mv, b.Moves[i].PP)
				if err != nil {
					if fallbackAttack < 0 {
						fallbackAttack = i
					}
					continue
				}
				if !haveEval || combat.BetterMove(eval, bestEval) {
					bestAttack, bestEval, haveEval = i, eval, true
				}
			case mv.Effect == rom.DefenseDown1Effect && defenseDown < 0:
				defenseDown = i
			}
		}

		// Only spend a turn on setup while we are actually behind, and only
		// while we can afford it. Below half HP the damage race is on and
		// another non-damaging turn loses it.
		//
		// Lowering the opponent's ATTACK was tried here too and removed.
		// MEASURED: it cost Charmander three extra turns and did not save
		// Bulbasaur, because Gen 1 critical hits ignore stat stages entirely.
		behind := b.OffenceStage() < 0
		healthy := b.ActiveMaxHP == 0 || b.ActiveHP*2 > b.ActiveMaxHP
		if behind && healthy && defenseDown >= 0 {
			if zbatDebug {
				fmt.Printf("zbat policy=setup slot=%d reason=physical-offence-stage-%d\n", defenseDown, b.OffenceStage())
			}
			return defenseDown
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
		return usable[0]
	}
}
