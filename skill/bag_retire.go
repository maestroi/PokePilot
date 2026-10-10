package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	itemfinderItem uint8 = 0x47
	oldRodItem     uint8 = 0x4C
	goodRodItem    uint8 = 0x4D
	superRodItem   uint8 = 0x4E
)

var rodItems = map[uint8]string{oldRodItem: rom.RodOld, goodRodItem: rom.RodGood, superRodItem: rom.RodSuper}

// RetiredBagItems lists bag items with no remaining use for this run, in bag
// order. They are stored in the Player PC (reversibly) before anything is
// tossed. Only provably dead items qualify: the S.S. Ticket after the ship
// sails, the unused Itemfinder, a rod whose every ROM fishing species is
// already owned, and an HM whose move the party can already use (HM
// ownership counts the PC, and repair withdraws it to re-teach). Items that
// still guard pickups or catches (Card Key, Lift Key, Silph Scope) stay.
// run-d6dokr184ky81 sat at 20/20 with several of these, so every new-kind
// pickup was filtered out.
func RetiredBagItems(romData []byte, mem *state.Mem) []uint8 {
	var out []uint8
	var owned map[uint8]bool
	learnedHM := map[uint8]bool{}
	for _, move := range ProgressionFieldMoves() {
		if spec, ok := FieldMoveSpecFor(move); ok && FieldCapabilityFor(mem, move).Usable {
			learnedHM[spec.HMItem] = true
		}
	}
	var hms []uint8
	for _, it := range state.DecodeInventory(mem).Items {
		switch {
		case learnedHM[it.ID]:
			hms = append(hms, it.ID)
		case it.ID == ssTicketItem && state.HasEvent(mem, state.EventSSAnneLeft),
			it.ID == itemfinderItem:
			out = append(out, it.ID)
		case rodItems[it.ID] != "":
			if owned == nil {
				owned = map[uint8]bool{}
				for _, dex := range state.DecodePokedex(mem).Owned {
					owned[dex] = true
				}
			}
			if rodYieldOwned(romData, rodItems[it.ID], owned) {
				out = append(out, it.ID)
			}
		}
	}
	// HMs go last: unlike dead items, re-teaching one costs a Center trip.
	return append(out, hms...)
}

func rodYieldOwned(romData []byte, rod string, owned map[uint8]bool) bool {
	encounters, err := rom.FishingEncounters(romData)
	if err != nil {
		return false
	}
	any := false
	for _, e := range encounters {
		if e.Rod != rod {
			continue
		}
		dex, err := rom.InternalSpeciesDexNumber(romData, e.Species)
		if err != nil || !owned[dex] {
			return false
		}
		any = true
	}
	return any
}

func storeRetiredItemForBagSpace(m *emu.Emu, romData []byte, policy MovePolicy) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.HasEvent(&mem, eventInSafariZone) {
		return false, nil
	}
	retired := RetiredBagItems(romData, &mem)
	if len(retired) == 0 {
		return false, nil
	}
	err := runInventoryDetour(m, romData, policy, func() error {
		return DepositBagStack(m, romData, policy, retired[0])
	})
	if err != nil {
		return false, fmt.Errorf("skill: bag pressure: store retired item %#02x in Player PC: %w", retired[0], err)
	}
	return true, nil
}

// withdrawStoredHM puts move's HM back in the bag when only the Player PC
// holds it, so the caller can teach it. A bag HM, or none at all, is a no-op.
func withdrawStoredHM(m *emu.Emu, romData []byte, policy MovePolicy, move FieldMove) error {
	spec, ok := FieldMoveSpecFor(move)
	if !ok {
		return nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, qty := bagEntry(&mem, spec.HMItem); qty > 0 ||
		pcItemQuantity(state.DecodePCItemStorage(&mem).Items, spec.HMItem) == 0 {
		return nil
	}
	if err := EnsureBagSpaceFor(m, spec.HMItem); err != nil {
		return fmt.Errorf("make bag room to withdraw %s: %w", spec.Name, err)
	}
	if err := runInventoryDetour(m, romData, policy, func() error {
		return WithdrawPCItem(m, romData, policy, spec.HMItem, 1)
	}); err != nil {
		return fmt.Errorf("withdraw %s from Player PC: %w", spec.Name, err)
	}
	return nil
}
