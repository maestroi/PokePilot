package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

// chooseBattleMedicineState returns one conservative medicine action for the
// active mon using only profile-projected roster/inventory state.
func chooseBattleMedicineState(resources game.BattleResourcesState) (battleMedicineChoice, bool) {
	if !resources.InBattle {
		return battleMedicineChoice{}, false
	}
	slot := resources.ActiveSlot
	if slot < 0 || slot >= len(resources.Party) {
		return battleMedicineChoice{}, false
	}
	mon := resources.Party[slot]
	if mon.Fainted() || mon.MaxHP == 0 {
		return battleMedicineChoice{}, false
	}

	lowHP := mon.HP*3 <= mon.MaxHP
	if lowHP && mon.Status != "" {
		if item, ok := chooseFullRestoreState(resources, mon.Status); ok {
			return battleMedicineChoice{
				Item:   item,
				Slot:   slot,
				Reason: fmt.Sprintf("active HP %d/%d and %s", mon.HP, mon.MaxHP, mon.Status),
			}, true
		}
	}
	if lowHP {
		if item, ok := chooseHPMedicineState(resources, int(mon.MaxHP-mon.HP)); ok {
			return battleMedicineChoice{
				Item:   item,
				Slot:   slot,
				Reason: fmt.Sprintf("active HP %d/%d", mon.HP, mon.MaxHP),
			}, true
		}
	}
	if mon.Status != "" {
		if item, ok := chooseStatusMedicineState(resources, mon.Status); ok {
			return battleMedicineChoice{Item: item, Slot: slot, Reason: "active is " + mon.Status}, true
		}
	}
	return battleMedicineChoice{}, false
}

// chooseFullRestoreState finds an owned medicine that both restores full HP
// and cures status, resolving low HP and status in a single turn.
func chooseFullRestoreState(resources game.BattleResourcesState, status string) (uint8, bool) {
	for _, med := range resources.Medicines {
		if med.HP == game.MedicineFullHP && med.CuresStatus(status) && resourceBagHas(resources, med) {
			return uint8(med.NativeItemID), true
		}
	}
	return 0, false
}

// chooseHPMedicineState returns the smallest owned heal covering missing HP,
// else the strongest owned heal. The profile orders Medicines by HP.
func chooseHPMedicineState(resources game.BattleResourcesState, missing int) (uint8, bool) {
	var strongest uint8
	found := false
	for _, med := range resources.Medicines {
		if med.HP <= 0 || !resourceBagHas(resources, med) {
			continue
		}
		strongest = uint8(med.NativeItemID)
		found = true
		if med.HP >= missing {
			return strongest, true
		}
	}
	return strongest, found
}

// chooseStatusMedicineState prefers the specific cure, then a pure
// all-status cure, and only then a cure that also restores HP.
func chooseStatusMedicineState(resources game.BattleResourcesState, status string) (uint8, bool) {
	best, bestRank := uint8(0), 3
	for _, med := range resources.Medicines {
		if !med.CuresStatus(status) || !resourceBagHas(resources, med) {
			continue
		}
		rank := 2
		switch {
		case med.Cures == status:
			rank = 0
		case med.HP == 0:
			rank = 1
		}
		if rank < bestRank {
			best, bestRank = uint8(med.NativeItemID), rank
		}
	}
	return best, bestRank < 3
}

func resourceBagHas(resources game.BattleResourcesState, med game.Medicine) bool {
	return resources.ItemQuantity(med.NativeItemID) > 0
}
