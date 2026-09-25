package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// UseBattleMedicine uses one medicine item on one party slot through semantic
// battle/list/party-menu capabilities. Success is proven from profile-projected
// roster and inventory state: the target changes and the item count drops.
func UseBattleMedicine(m *emu.Emu, item uint8, slot int) error {
	resourcesDecoder, err := battleResourcesDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}
	listDecoder, err := listMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}
	partyDecoder, err := partyMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}
	runtimeDecoder, err := battleRuntimeDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}

	before := resourcesDecoder.DecodeBattleResources(m)
	if !before.InBattle {
		return fmt.Errorf("skill: UseBattleMedicine: no battle in progress on %s",
			battleRuntimeContext(runtimeDecoder.DecodeBattleRuntime(m)))
	}
	if slot < 0 || slot >= len(before.Party) {
		return fmt.Errorf("skill: UseBattleMedicine: slot %d out of range for a party of %d", slot, len(before.Party))
	}
	beforeMon := before.Party[slot]
	idx, ok := before.ItemIndex(uint16(item))
	beforeQty := before.ItemQuantity(uint16(item))
	if !ok || beforeQty < 1 {
		return fmt.Errorf("skill: UseBattleMedicine: %w (id %#02x)", ErrNotInBag, item)
	}

	if err := waitBattleMainMenu(m); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}
	if err := selectItemEntry(m); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: select ITEM: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		live := listDecoder.DecodeListMenu(m)
		return live.Visible && live.Kind == game.ListMenuItems
	}); err != nil {
		live := runtimeDecoder.DecodeBattleRuntime(m)
		return fmt.Errorf("skill: UseBattleMedicine: item list did not open within %d frames on %s",
			bagMenuBudget, battleRuntimeContext(live))
	}
	if err := selectScrollingListEntryWithDecoder(m, listDecoder, idx); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: select bag entry %d: %w", idx, err)
	}
	if _, err := m.StepUntil(itemUsePartyBudget, func(m *emu.Emu) bool {
		live := partyDecoder.DecodePartyMenu(m)
		return live.Visible && live.Kind == game.PartyMenuItemUse
	}); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: medicine target menu did not open within %d frames", itemUsePartyBudget)
	}
	if err := SelectPartySlot(m, slot); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}

	effectObserved := false
	start := m.FrameCount()
	for int(m.FrameCount()-start) <= bagUseBudget {
		after := resourcesDecoder.DecodeBattleResources(m)
		if slot < len(after.Party) {
			afterMon := after.Party[slot]
			if afterMon.HP > beforeMon.HP || (beforeMon.Status != "" && afterMon.Status == "") {
				effectObserved = true
			}
		}
		afterQty := after.ItemQuantity(uint16(item))
		if afterQty == beforeQty-1 {
			if !effectObserved {
				return fmt.Errorf("skill: UseBattleMedicine: item %#02x was consumed but slot %d showed no HP/status effect", item, slot)
			}
			return nil
		}
		if !after.InBattle {
			return fmt.Errorf("skill: UseBattleMedicine: battle ended before item %#02x consumption/effect was verified", item)
		}
		m.Tap(emu.A, 3, 7)
	}
	afterQty := resourcesDecoder.DecodeBattleResources(m).ItemQuantity(uint16(item))
	return fmt.Errorf("skill: UseBattleMedicine: item %#02x did not complete within %d frames (bag %d -> %d, effect=%t)",
		item, bagUseBudget, beforeQty, afterQty, effectObserved)
}
