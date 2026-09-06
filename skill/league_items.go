package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// Gen 1 revive item ids from pokered/constants/item_constants.asm.
const (
	itemRevive    uint8 = 0x35
	itemMaxRevive uint8 = 0x36
)

// RevivePartyMember revives a fainted party member from the overworld through
// the ordinary START -> ITEM -> party menu flow. UseFieldItem already proves
// that HP rose and the bag count dropped by exactly one; this wrapper adds the
// stronger semantic contract that the target was fainted before the action and
// is live afterwards.
func RevivePartyMember(m *emu.Emu, slot int, max bool) error {
	var before state.Mem
	state.Snapshot(m, &before)
	party := state.DecodeParty(&before)
	if slot < 0 || slot >= len(party.Mons) {
		return fmt.Errorf("skill: RevivePartyMember: slot %d out of range for party of %d", slot, len(party.Mons))
	}
	if !party.Mons[slot].Fainted() {
		return fmt.Errorf("skill: RevivePartyMember: slot %d is not fainted (HP %d/%d)", slot, party.Mons[slot].HP, party.Mons[slot].MaxHP)
	}

	item := itemRevive
	if max {
		item = itemMaxRevive
	}
	if err := UseFieldItem(m, item, slot); err != nil {
		return fmt.Errorf("skill: RevivePartyMember: %w", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	afterParty := state.DecodeParty(&after)
	if slot >= len(afterParty.Mons) || afterParty.Mons[slot].Fainted() {
		return fmt.Errorf("skill: RevivePartyMember: item %#02x completed but slot %d is still fainted", item, slot)
	}
	if max && afterParty.Mons[slot].HP != afterParty.Mons[slot].MaxHP {
		return fmt.Errorf("skill: RevivePartyMember: MAX REVIVE left slot %d at %d/%d HP", slot, afterParty.Mons[slot].HP, afterParty.Mons[slot].MaxHP)
	}
	return nil
}

// RestorePartyPP uses one Ether/Elixer-family item on a party member and
// positively verifies that at least one current PP value increased. For ETHER
// and MAX ETHER, UseFieldItem deterministically chooses the emptiest known
// move, preferring an exhausted move; ELIXER/MAX ELIXER restore all moves on
// the selected member. Bag consumption is independently verified by
// UseFieldItem.
func RestorePartyPP(m *emu.Emu, item uint8, slot int) error {
	if !isPPRestoreItem(item) {
		return fmt.Errorf("skill: RestorePartyPP: item %#02x is not a PP restore item", item)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	party := state.DecodeParty(&before)
	if slot < 0 || slot >= len(party.Mons) {
		return fmt.Errorf("skill: RestorePartyPP: slot %d out of range for party of %d", slot, len(party.Mons))
	}
	beforePP := party.Mons[slot].PP
	if err := UseFieldItem(m, item, slot); err != nil {
		return fmt.Errorf("skill: RestorePartyPP: %w", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	afterParty := state.DecodeParty(&after)
	if slot >= len(afterParty.Mons) {
		return fmt.Errorf("skill: RestorePartyPP: party slot %d disappeared after item use", slot)
	}
	for i := range beforePP {
		if afterParty.Mons[slot].PP[i] > beforePP[i] {
			return nil
		}
	}
	return fmt.Errorf("skill: RestorePartyPP: item %#02x completed but slot %d PP did not increase: %v -> %v",
		item, slot, beforePP, afterParty.Mons[slot].PP)
}
