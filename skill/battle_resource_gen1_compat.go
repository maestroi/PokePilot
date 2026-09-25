package skill

import "github.com/maestroi/pokepilot/red/state"

// Gen-I compatibility wrappers retain historical unit-test/helper entry points
// while routing decisions through the portable resource policy.
// chooseBattleMedicine returns one conservative medicine action for the
// active mon. HP medicine is considered only at one-third HP or below;
// status medicine is considered whenever a matching cure exists. When both
// apply, FULL RESTORE resolves them in one turn if available.
func chooseBattleMedicine(mem *state.Mem) (battleMedicineChoice, bool) {
	return chooseBattleMedicineState(gen1BattleResourcesFromMem(mem))
}

func chooseHPMedicine(mem *state.Mem, missing int) (uint8, bool) {
	var strongest uint8
	found := false
	for _, med := range hpMedicines {
		if !bagHasItem(mem, med.item) {
			continue
		}
		strongest = med.item
		found = true
		if med.heal >= missing {
			return med.item, true
		}
	}
	return strongest, found
}

func chooseStatusMedicine(mem *state.Mem, status string) (uint8, bool) {
	specific, ok := statusCure(status)
	if !ok {
		return 0, false
	}
	for _, item := range []uint8{specific, itemFullHeal, itemFullRestore} {
		if bagHasItem(mem, item) {
			return item, true
		}
	}
	return 0, false
}

func bagHasItem(mem *state.Mem, item uint8) bool {
	_, qty := bagEntry(mem, item)
	return qty > 0
}

// ppRecoverySlot returns the first live bench mon that has at least one known
// move with current PP. This is a dead-turn escape, not team strategy.
func ppRecoverySlot(mem *state.Mem) (int, bool) {
	return gen1BattleResourcesFromMem(mem).PPRecoverySlot()
}

func monHasCurrentPP(mon state.Mon) bool {
	for i, id := range mon.Moves {
		if id != 0 && mon.PP[i] > 0 {
			return true
		}
	}
	return false
}

func livePartyHasCurrentPP(mem *state.Mem) bool {
	return gen1BattleResourcesFromMem(mem).LivePartyHasCurrentPP()
}


