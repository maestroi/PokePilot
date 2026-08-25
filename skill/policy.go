package skill

import (
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

// StatAwareMove is the default policy for a real fight. It attacks with the
// strongest move it has, except when the opponent has been grinding our
// offence down, in which case it spends a turn lowering the opponent's
// Defense instead.
//
// This exists because FirstUsableMove loses the rival battle in Oak's lab,
// and not by chance. MEASURED: the rival's Bulbasaur opens with GROWL, four
// times in one observed fight. Each GROWL costs a stage of our Attack, and
// Squirtle's Tackle decays 3 -> 2 -> 2 -> 2 -> 1 -> 1 damage while Bulbasaur
// keeps hitting for 3. Bulbasaur is also the faster mon (45 to 43), so even
// an undisturbed Tackle race is lost by a turn. A policy that only ever
// attacks has no reply to this.
//
// The reply is a Defense-lowering move: in Gen 1 damage scales on the ratio
// of our Attack stage to their Defense stage, so -1 to their Defense cancels
// -1 from our Attack exactly. Squirtle's Tail Whip is the early instance.
//
// It closes over the ROM because move power and effect live in the ROM's
// move table, not in RAM, and MovePolicy is handed only the RAM state. A
// move id that is not in the table is treated as an attack, which is the
// safe reading: worst case we hit something.
//
// This is deliberately a plain heuristic, not a damage model. It is the
// stand-in for the planner that docs/DESIGN.md puts behind this same seam.
func StatAwareMove(romData []byte) MovePolicy {
	return func(b state.BattleState) int {
		usable := b.Usable()
		if len(usable) == 0 {
			return -1
		}

		bestAttack, bestPower := -1, -1
		defenseDown := -1
		for _, i := range usable {
			mv, err := rom.LookupMove(romData, b.Moves[i].ID)
			if err != nil {
				if bestAttack < 0 {
					bestAttack = i
				}
				continue
			}
			switch {
			case mv.Power > 0:
				if int(mv.Power) > bestPower {
					bestAttack, bestPower = i, int(mv.Power)
				}
			case mv.Effect == rom.DefenseDown1Effect && defenseDown < 0:
				defenseDown = i
			}
		}

		// Only spend a turn on setup while we are actually behind, and only
		// while we can afford it. Below half HP the race is on and another
		// non-damaging turn loses it.
		// Only spend a turn on setup while we are behind and can still
		// afford it. Below half HP the race is on and another non-damaging
		// turn loses it.
		//
		// Lowering the opponent's ATTACK was tried here too and removed.
		// MEASURED: it cost Charmander three extra turns and did not save
		// Bulbasaur, because Gen 1 critical hits ignore stat stages
		// entirely — a -3 Attack Charmander still took Bulbasaur from 7 HP
		// to 1 in a single turn. Only the Defense-lowering half pays for
		// its tempo.
		behind := b.OffenceStage() < 0
		healthy := b.ActiveMaxHP == 0 || b.ActiveHP*2 > b.ActiveMaxHP
		if behind && healthy && defenseDown >= 0 {
			return defenseDown
		}
		if bestAttack >= 0 {
			return bestAttack
		}
		return usable[0]
	}
}
