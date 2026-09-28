package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

const (
	// A bench heal deliberately spends the active mon's turn instead of
	// protecting it, so require the recovered target to be materially stronger
	// in the current matchup. This reuses the same 50% material-gain bar as a
	// tactical switch, but evaluates the target after the selected medicine.
	sacrificeHealGainNumerator   int64 = 3
	sacrificeHealGainDenominator int64 = 2
)

// chooseSacrificialBenchHealState uses a low-value active mon as a recovery
// window for a critically weak, materially stronger bench member. The target
// must remain alive, ordinary battle medicine must move it out of the critical
// HP band in one turn, and its post-heal matchup score must beat the active
// mon's full-HP baseline by the same material-gain threshold used for switches.
//
// The policy is intentionally generic over the portable battle-resource state
// and generation combat strategy. Item identity/effects remain owned by the
// existing medicine adapter.
func chooseSacrificialBenchHealState(
	romData []byte,
	resources game.BattleResourcesState,
	b game.BattleState,
) (battleMedicineChoice, bool) {
	strategy, err := combatStrategyForROM(romData)
	if err != nil {
		return battleMedicineChoice{}, false
	}
	return chooseSacrificialBenchHealWithStrategyContext(strategy, romData, resources, b, BattleSequenceContext{})
}

func chooseSacrificialBenchHealStateWithContext(
	romData []byte,
	resources game.BattleResourcesState,
	b game.BattleState,
	context BattleSequenceContext,
) (battleMedicineChoice, bool) {
	strategy, err := combatStrategyForROM(romData)
	if err != nil {
		return battleMedicineChoice{}, false
	}
	return chooseSacrificialBenchHealWithStrategyContext(strategy, romData, resources, b, context)
}

func chooseSacrificialBenchHealWithStrategy(
	strategy game.BattleCombatStrategy,
	romData []byte,
	resources game.BattleResourcesState,
	b game.BattleState,
) (battleMedicineChoice, bool) {
	return chooseSacrificialBenchHealWithStrategyContext(strategy, romData, resources, b, BattleSequenceContext{})
}

func chooseSacrificialBenchHealWithStrategyContext(
	strategy game.BattleCombatStrategy,
	romData []byte,
	resources game.BattleResourcesState,
	b game.BattleState,
	context BattleSequenceContext,
) (battleMedicineChoice, bool) {
	activeSlot := resources.ActiveSlot
	if strategy == nil || !resources.InBattle || activeSlot < 0 || activeSlot >= len(resources.Party) {
		return battleMedicineChoice{}, false
	}
	active := resources.Party[activeSlot]
	if active.Fainted() || active.MaxHP == 0 {
		return battleMedicineChoice{}, false
	}
	if context.MinimumViableParty > 0 &&
		sequenceViablePartyCount(resources.Party) <= context.MinimumViableParty {
		return battleMedicineChoice{}, false
	}
	futureValues := futurePreservationValues(strategy, romData, resources.Party, context)

	// Compare against what the active mon would be worth at full HP. That keeps
	// "sacrifice" about party role/strength rather than merely preferring any
	// other mon because the current one happens to be damaged.
	activeBaseline := b
	if activeBaseline.ActiveMaxHP > 0 {
		activeBaseline.ActiveHP = activeBaseline.ActiveMaxHP
	}
	activeEval := addFutureValue(evaluateActiveForSwitch(strategy, romData, resources.Party, activeSlot, activeBaseline), futureValues)

	_, defender := battleCombatants(b)
	var best battleMedicineChoice
	var bestEval switchEvaluation
	found := false

	for slot, mon := range resources.Party {
		if slot == activeSlot || mon.Fainted() || mon.MaxHP == 0 || !criticallyWeak(mon) {
			continue
		}

		item, ok := chooseHPMedicineState(resources, int(mon.MaxHP-mon.HP))
		if !ok {
			continue
		}
		projected := projectBattleHPMedicine(mon, item)
		// Do not spend a sacrificial turn on a heal too small to make the carry
		// a viable switch-in afterward.
		if projected.HP <= mon.HP || criticallyWeak(projected) {
			continue
		}

		eval := addFutureValue(evaluatePartyMonForSwitch(strategy, romData, slot, projected, defender), futureValues)
		if eval.BestMoveSlot < 0 || eval.BestMove.ExpectedScore <= 0 {
			continue
		}
		if activeEval.FutureValue >= eval.FutureValue+futureSacrificeProtectionGap {
			continue
		}
		if eval.Score*sacrificeHealGainDenominator <= activeEval.Score*sacrificeHealGainNumerator {
			continue
		}
		if found && !betterSwitchEvaluation(eval, bestEval) {
			continue
		}

		bestEval = eval
		best = battleMedicineChoice{
			Item: item,
			Slot: slot,
			Reason: fmt.Sprintf(
				"sacrifice-heal active slot %d baseline-score=%d future=%d to preserve stronger bench slot %d HP %d/%d->%d/%d score=%d future=%d",
				activeSlot, activeEval.Score, activeEval.FutureValue, slot, mon.HP, mon.MaxHP, projected.HP, projected.MaxHP, eval.Score, eval.FutureValue,
			),
		}
		found = true
	}

	return best, found
}

func projectBattleHPMedicine(mon game.BattlePartyMon, item uint8) game.BattlePartyMon {
	for _, med := range hpMedicines {
		if med.item != item {
			continue
		}
		hp := int(mon.HP) + med.heal
		if hp > int(mon.MaxHP) {
			hp = int(mon.MaxHP)
		}
		mon.HP = uint16(hp)
		break
	}
	if item == itemFullRestore {
		mon.Status = ""
	}
	return mon
}
