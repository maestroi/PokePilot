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

	battleMenu, err := battleMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}
	if err := waitBattleMainMenuWithDecoder(m, battleMenu); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}
	if err := activateBattleMainMenuEntryWithDecoder(m, battleMenu, game.BattleMenuItems, bagMenuBudget, func() bool {
		live := listDecoder.DecodeListMenu(m)
		return live.Visible && live.Kind == game.ListMenuItems
	}); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: open item list on %s: %w",
			battleRuntimeContext(runtimeDecoder.DecodeBattleRuntime(m)), err)
	}
	// Gold/Silver PACK remembers its last pocket; Bag carries the pocket.
	if err := selectItemPocketWithDecoder(m, listDecoder, before.Bag[idx].Pocket); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}
	if err := selectScrollingListEntryWithDecoder(m, listDecoder, idx); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: select bag entry %d: %w", idx, err)
	}
	// Some profiles (Gold/Silver BattlePack) ask USE/QUIT before the target
	// menu. Confirm only a rendered prompt whose cursor is on USE.
	field, _ := fieldItemDecoderFor(m)
	usePrompt := func() bool {
		if field == nil {
			return false
		}
		prompt := field.DecodeFieldItem(m)
		return prompt.UsePromptVisible && prompt.UseSelected
	}
	targetMenu := func() bool {
		live := partyDecoder.DecodePartyMenu(m)
		return live.Visible && live.Kind == game.PartyMenuItemUse
	}
	for confirms := 0; ; confirms++ {
		if _, err := m.StepUntil(itemUsePartyBudget, func(*emu.Emu) bool {
			return targetMenu() || usePrompt()
		}); err != nil {
			return fmt.Errorf("skill: UseBattleMedicine: medicine target menu did not open within %d frames", itemUsePartyBudget)
		}
		if targetMenu() {
			break
		}
		if confirms >= 2 {
			return fmt.Errorf("skill: UseBattleMedicine: USE prompt did not lead to the medicine target menu")
		}
		m.Tap(emu.A, 3, 7)
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
			return closeMedicineResult(m, partyDecoder, runtimeDecoder)
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

// closeMedicineResult waits for the target menu to close. Gold/Silver print
// "… health returned." over the party screen; returning while it is up makes
// the battle loop read it as an unbidden party menu and pick a slot. Only
// text is advanced here; a party slot is never selected.
func closeMedicineResult(m *emu.Emu, party game.PartyMenuDecoder, runtime game.BattleRuntimeDecoder) error {
	start := m.FrameCount()
	for int(m.FrameCount()-start) <= bagUseBudget {
		if !party.DecodePartyMenu(m).Visible {
			return nil
		}
		if runtime.DecodeBattleRuntime(m).TextActive {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("skill: UseBattleMedicine: medicine result screen did not close within %d frames", bagUseBudget)
}
