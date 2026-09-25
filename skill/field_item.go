package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// ErrFieldItemNoEffect reports that the field item's sequence ran to
// completion but the target did not gain HP, clear status, gain PP, or level.
var ErrFieldItemNoEffect = errors.New("skill: UseFieldItem: the item had no effect on the target")

// ErrFieldItemPrompt reports an unexpected gameplay choice while paging the
// result. Generic cleanup never answers such choices implicitly.
var ErrFieldItemPrompt = errors.New("skill: UseFieldItem: a two-option prompt appeared while paging the result text; not answering it")

const (
	useTossBudget         = 60
	itemUsePartyBudget    = 1000
	itemUseMoveBudget     = 1000
	fieldResultTextBudget = 3000
)

type fieldItemMachine interface {
	menuMachine
	FrameCount() uint64
}

// UseFieldItem uses one carried item on a party member from the overworld.
// Menu layout, item action prompts, move-target menus, party RAM and item
// semantics are all owned by the active profile. Success requires both a
// positive target effect and exactly one item consumed.
func UseFieldItem(m *emu.Emu, item uint8, slot int) error {
	field, err := fieldItemDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseFieldItem: %w", err)
	}
	inventory, err := inventoryDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseFieldItem: %w", err)
	}
	menu, err := menuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseFieldItem: %w", err)
	}
	list, err := listMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseFieldItem: %w", err)
	}
	party, err := partyMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseFieldItem: %w", err)
	}
	return useFieldItemWithDecoders(m, uint16(item), slot, field, inventory, menu, list, party)
}

func useFieldItemWithDecoders(
	m fieldItemMachine,
	item uint16,
	slot int,
	field game.FieldItemDecoder,
	inventory game.InventoryDecoder,
	menu game.MenuDecoder,
	list game.ListMenuDecoder,
	party game.PartyMenuDecoder,
) error {
	if m == nil || field == nil || inventory == nil || menu == nil || list == nil || party == nil {
		return fmt.Errorf("skill: UseFieldItem: incomplete semantic execution capability")
	}
	live := field.DecodeFieldItem(m)
	if !live.OverworldReady {
		return fmt.Errorf("skill: UseFieldItem: player not at a controllable overworld boundary")
	}
	if slot < 0 || slot >= len(live.Party) {
		return fmt.Errorf("skill: UseFieldItem: slot %d out of range for a party of %d", slot, len(live.Party))
	}
	before := live.Party[slot]
	semantics := field.FieldItemSemantics(item)
	moveSlot := -1
	if semantics.SingleMoveTarget {
		var ok bool
		moveSlot, ok = ppRestoreMoveSlotState(before)
		if !ok {
			return fmt.Errorf("skill: UseFieldItem: PP restore target slot %d has no known moves", slot)
		}
	}
	idx, bagBefore := fieldItemInventoryEntry(inventory.DecodeInventory(m), item)
	if idx < 0 {
		return fmt.Errorf("skill: UseFieldItem: %w (id %#04x)", ErrNotInBag, item)
	}

	if err := openStartMenuEntryWithDecoder(m, menu, game.StartMenuItems); err != nil {
		return fmt.Errorf("skill: UseFieldItem: open ITEM: %w", err)
	}
	if !waitMenuUntil(m, bagMenuBudget, func() bool {
		s := list.DecodeListMenu(m)
		return s.Visible && s.Kind == game.ListMenuItems
	}) {
		return fmt.Errorf("skill: UseFieldItem: item list did not open after ITEM")
	}

	for attempt := 0; ; attempt++ {
		if err := selectScrollingListEntryWithDecoder(m, list, idx); err != nil {
			return fmt.Errorf("skill: UseFieldItem: select bag entry: %w", err)
		}
		if waitMenuUntil(m, useTossBudget, func() bool {
			return field.DecodeFieldItem(m).UsePromptVisible
		}) {
			break
		}
		if attempt >= 2 {
			return fmt.Errorf("skill: UseFieldItem: USE action prompt did not appear after selecting the bag entry (3 attempts): %s",
				field.DecodeFieldItem(m).DebugText)
		}
	}
	live = field.DecodeFieldItem(m)
	if !live.UsePromptVisible || !live.UseSelected {
		return fmt.Errorf("skill: UseFieldItem: item action cursor is not on USE")
	}

	for attempt := 0; ; attempt++ {
		m.Tap(emu.A, 3, 7)
		if waitMenuUntil(m, itemUsePartyBudget, func() bool {
			s := party.DecodePartyMenu(m)
			return s.Visible && s.Kind == game.PartyMenuItemUse
		}) {
			break
		}
		if !field.DecodeFieldItem(m).UsePromptVisible || attempt >= 2 {
			return fmt.Errorf("skill: UseFieldItem: item-use party menu did not appear after USE: %s",
				field.DecodeFieldItem(m).DebugText)
		}
	}
	if err := selectPartySlotWithDecoder(m, party, slot); err != nil {
		return fmt.Errorf("skill: UseFieldItem: %w", err)
	}

	if semantics.SingleMoveTarget {
		if !waitMenuUntil(m, itemUseMoveBudget, func() bool {
			return field.DecodeFieldItem(m).MoveMenuVisible
		}) {
			return fmt.Errorf("skill: UseFieldItem: PP move menu did not appear for item %#04x slot %d: %s",
				item, slot, field.DecodeFieldItem(m).DebugText)
		}
		if err := selectFieldItemMoveSlot(m, field, moveSlot); err != nil {
			return fmt.Errorf("skill: UseFieldItem: select move slot %d for PP restore: %w", moveSlot, err)
		}
	}

	live = field.DecodeFieldItem(m)
	if !live.OverworldReady && !live.ResultTextActive {
		if !waitMenuUntil(m, 500, func() bool {
			s := field.DecodeFieldItem(m)
			return s.ResultTextActive || s.OverworldReady
		}) {
			return fmt.Errorf("skill: UseFieldItem: result text did not appear within 500 frames: %s",
				field.DecodeFieldItem(m).DebugText)
		}
	}

	start := m.FrameCount()
	for {
		live = field.DecodeFieldItem(m)
		if live.OverworldReady {
			break
		}
		if live.InBattle {
			return fmt.Errorf("skill: UseFieldItem: unexpected battle while closing item UI")
		}
		if live.ChoiceVisible {
			return fmt.Errorf("%w: %s", ErrFieldItemPrompt, live.DebugText)
		}
		if int(m.FrameCount()-start) > fieldResultTextBudget {
			return fmt.Errorf("skill: UseFieldItem: not back to the overworld after item use: %s", live.DebugText)
		}
		m.Tap(emu.B, 3, 7)
	}

	live = field.DecodeFieldItem(m)
	if slot >= len(live.Party) {
		return fmt.Errorf("skill: UseFieldItem: party slot %d disappeared after item use", slot)
	}
	after := live.Party[slot]
	if !fieldItemHadEffectState(before, after) {
		return fmt.Errorf("%w: slot %d level %d->%d HP %d/%d -> %d/%d, status %q -> %q, PP %v -> %v (item %#04x, ppRestore=%v)",
			ErrFieldItemNoEffect, slot, before.Level, after.Level, before.HP, before.MaxHP, after.HP, after.MaxHP,
			before.Status, after.Status, before.PP, after.PP, item, semantics.PPRestore)
	}
	_, bagAfter := fieldItemInventoryEntry(inventory.DecodeInventory(m), item)
	if bagAfter != bagBefore-1 {
		return fmt.Errorf("skill: UseFieldItem: bag count for %#04x did not drop from %d (now %d)", item, bagBefore, bagAfter)
	}
	return nil
}
