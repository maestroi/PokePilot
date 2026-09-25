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
	if lowHP && mon.Status != "" && resourceBagHasItem(resources, itemFullRestore) {
		return battleMedicineChoice{
			Item:   itemFullRestore,
			Slot:   slot,
			Reason: fmt.Sprintf("active HP %d/%d and %s", mon.HP, mon.MaxHP, mon.Status),
		}, true
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

func chooseHPMedicineState(resources game.BattleResourcesState, missing int) (uint8, bool) {
	var strongest uint8
	found := false
	for _, med := range hpMedicines {
		if !resourceBagHasItem(resources, med.item) {
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

func chooseStatusMedicineState(resources game.BattleResourcesState, status string) (uint8, bool) {
	specific, ok := statusCure(status)
	if !ok {
		return 0, false
	}
	for _, item := range []uint8{specific, itemFullHeal, itemFullRestore} {
		if resourceBagHasItem(resources, item) {
			return item, true
		}
	}
	return 0, false
}

func resourceBagHasItem(resources game.BattleResourcesState, item uint8) bool {
	return resources.ItemQuantity(uint16(item)) > 0
}
