package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const repelUseSettleBudget = 1200

// UseRepel activates one repel-family item from the overworld. The active
// profile owns native item identity and the effect counter; success requires
// both the semantic repel duration and exactly one consumed item.
func UseRepel(m *emu.Emu, item uint8) error {
	field, err := fieldItemDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseRepel: %w", err)
	}
	inventory, err := inventoryDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseRepel: %w", err)
	}
	menu, err := menuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseRepel: %w", err)
	}
	list, err := listMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseRepel: %w", err)
	}
	return useRepelWithDecoders(m, uint16(item), field, inventory, menu, list)
}

func useRepelWithDecoders(
	m fieldItemMachine,
	item uint16,
	field game.FieldItemDecoder,
	inventory game.InventoryDecoder,
	menu game.MenuDecoder,
	list game.ListMenuDecoder,
) error {
	if m == nil || field == nil || inventory == nil || menu == nil || list == nil {
		return fmt.Errorf("skill: UseRepel: incomplete semantic execution capability")
	}
	duration := field.FieldItemSemantics(item).RepelSteps
	if duration <= 0 {
		return fmt.Errorf("skill: UseRepel: item %#04x is not a Repel-family item", item)
	}
	live := field.DecodeFieldItem(m)
	if !live.OverworldReady {
		return fmt.Errorf("skill: UseRepel: player not at a controllable overworld boundary")
	}
	if live.RepelSteps > 0 {
		return fmt.Errorf("skill: UseRepel: repel is already active for %d more steps", live.RepelSteps)
	}
	idx, bagBefore := fieldItemInventoryEntry(inventory.DecodeInventory(m), item)
	if idx < 0 {
		return fmt.Errorf("skill: UseRepel: %w (id %#04x)", ErrNotInBag, item)
	}

	if err := openStartMenuEntryWithDecoder(m, menu, game.StartMenuItems); err != nil {
		return fmt.Errorf("skill: UseRepel: open ITEM: %w", err)
	}
	if !waitMenuUntil(m, bagMenuBudget, func() bool {
		s := list.DecodeListMenu(m)
		return s.Visible && s.Kind == game.ListMenuItems
	}) {
		return fmt.Errorf("skill: UseRepel: item list did not open")
	}
	if err := selectScrollingListEntryWithDecoder(m, list, idx); err != nil {
		return fmt.Errorf("skill: UseRepel: select bag entry: %w", err)
	}
	if !waitMenuUntil(m, useTossBudget, func() bool {
		return field.DecodeFieldItem(m).UsePromptVisible
	}) {
		return fmt.Errorf("skill: UseRepel: USE action prompt did not open")
	}
	live = field.DecodeFieldItem(m)
	if !live.UseSelected {
		return fmt.Errorf("skill: UseRepel: item action cursor is not on USE")
	}

	m.Tap(emu.A, 3, 7)
	if !waitMenuUntil(m, repelUseSettleBudget, func() bool {
		return field.DecodeFieldItem(m).RepelSteps == duration
	}) {
		live = field.DecodeFieldItem(m)
		return fmt.Errorf("skill: UseRepel: effect did not load %d steps: now=%d %s",
			duration, live.RepelSteps, live.DebugText)
	}
	_ = waitMenuUntil(m, useTossBudget, func() bool {
		return !field.DecodeFieldItem(m).UsePromptVisible
	})

	start := m.FrameCount()
	for {
		live = field.DecodeFieldItem(m)
		if live.OverworldReady {
			break
		}
		if live.InBattle {
			return fmt.Errorf("skill: UseRepel: unexpected battle while closing item UI")
		}
		if live.ChoiceVisible {
			return fmt.Errorf("skill: UseRepel: unexpected choice while closing item UI: %s", live.DebugText)
		}
		if int(m.FrameCount()-start) > fieldResultTextBudget {
			return fmt.Errorf("skill: UseRepel: item UI did not close: %s", live.DebugText)
		}
		m.Tap(emu.B, 3, 7)
	}

	live = field.DecodeFieldItem(m)
	if live.RepelSteps != duration {
		return fmt.Errorf("skill: UseRepel: active steps changed unexpectedly: got %d want %d", live.RepelSteps, duration)
	}
	_, bagAfter := fieldItemInventoryEntry(inventory.DecodeInventory(m), item)
	if bagAfter != bagBefore-1 {
		return fmt.Errorf("skill: UseRepel: bag count for %#04x did not drop from %d (now %d)", item, bagBefore, bagAfter)
	}
	return nil
}

// UseBestRepel activates the longest-duration repel stack exposed by the
// active profile. It is a no-op while repel is active or no preferred repel
// is carried.
func UseBestRepel(m *emu.Emu) (bool, error) {
	field, err := fieldItemDecoderFor(m)
	if err != nil {
		return false, fmt.Errorf("skill: UseBestRepel: %w", err)
	}
	inventory, err := inventoryDecoderFor(m)
	if err != nil {
		return false, fmt.Errorf("skill: UseBestRepel: %w", err)
	}
	if field.DecodeFieldItem(m).RepelSteps > 0 {
		return false, nil
	}
	menu, err := menuDecoderFor(m)
	if err != nil {
		return false, fmt.Errorf("skill: UseBestRepel: %w", err)
	}
	list, err := listMenuDecoderFor(m)
	if err != nil {
		return false, fmt.Errorf("skill: UseBestRepel: %w", err)
	}
	bag := inventory.DecodeInventory(m)
	for _, item := range field.PreferredRepels() {
		if _, quantity := fieldItemInventoryEntry(bag, item); quantity > 0 {
			if err := useRepelWithDecoders(m, item, field, inventory, menu, list); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}
