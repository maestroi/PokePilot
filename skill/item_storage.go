package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

var (
	ErrPCItemStorageFull = errors.New("skill: Player PC item storage is full")
	ErrPCItemNotStored   = errors.New("skill: requested item is not stored in Player PC")
)

func playersPCMenuScreen(mem *state.Mem) bool {
	text := state.ScreenText(mem)
	return mem.U8(sym.TopMenuItemX) == 1 && mem.U8(sym.TopMenuItemY) == 2 &&
		mem.U8(sym.MaxMenuItem) == 3 &&
		strings.Contains(text, "WITHDRAW ITEM") &&
		strings.Contains(text, "DEPOSIT ITEM") &&
		strings.Contains(text, "TOSS ITEM")
}

func playersPCMenuUp(mem *state.Mem) bool {
	return state.MenuUp(mem) && playersPCMenuScreen(mem)
}

func pcItemQuantity(items []state.BagItem, item uint8) int {
	for _, it := range items {
		if it.ID == item {
			return int(it.Quantity)
		}
	}
	return 0
}

func pcItemIndex(items []state.BagItem, item uint8) int {
	for i, it := range items {
		if it.ID == item {
			return i
		}
	}
	return -1
}

func openPlayersPC(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if err := ensureAtPokemonCenterPC(m, romData, policy); err != nil {
		return err
	}
	m.Tap(emu.A, 3, 7)
	if err := pcAdvanceUntil(m, nil, pcMainMenuUp, "PC main menu"); err != nil {
		return err
	}
	if err := SelectMenuItem(m, 1); err != nil { // MY PC / ITEM STORAGE SYSTEM
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Player PC: select MY PC: %w", err)
	}
	if err := pcAdvanceUntil(m, pcMainMenuUp, playersPCMenuUp, "Player PC item menu"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	return nil
}

func pcItemListUp(mem *state.Mem) bool {
	return state.MenuUp(mem) && mem.U8(sym.ListMenuID) == itemListMenuID &&
		mem.U8(sym.MenuWatchedKeys) == watchListOrQty
}

// DepositBagStack stores an entire bag stack in the Player PC Item Storage
// System through normal gameplay. It travels to a reachable Pokemon Center,
// verifies that the stack left the bag and appeared in PC storage, then closes
// the PC to a controllable overworld boundary.
func DepositBagStack(m *emu.Emu, romData []byte, policy MovePolicy, item uint8) error {
	if policy == nil {
		return fmt.Errorf("skill: Player PC: nil move policy")
	}
	var before state.Mem
	state.Snapshot(m, &before)
	idx, bagBefore := bagEntry(&before, item)
	if idx < 0 || bagBefore == 0 {
		return fmt.Errorf("skill: Player PC: item %#02x is not in the bag", item)
	}
	storedBefore := state.DecodePCItemStorage(&before)
	pcBefore := pcItemQuantity(storedBefore.Items, item)
	if len(storedBefore.Items) >= sym.PCItemCapacity && pcBefore == 0 {
		return fmt.Errorf("%w: %d/%d stacks", ErrPCItemStorageFull, len(storedBefore.Items), sym.PCItemCapacity)
	}
	if pcBefore+bagBefore > 99 {
		return fmt.Errorf("skill: Player PC: storing item %#02x x%d would overflow stored stack x%d", item, bagBefore, pcBefore)
	}

	if err := openPlayersPC(m, romData, policy); err != nil {
		return err
	}
	cleanup := func(err error) error {
		if closeErr := closePCToOverworld(m); closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}
	if err := SelectMenuItem(m, 1); err != nil { // DEPOSIT ITEM
		return cleanup(fmt.Errorf("skill: Player PC: select DEPOSIT ITEM: %w", err))
	}
	if err := pcAdvanceUntil(m, playersPCMenuUp, pcItemListUp, "bag list for item deposit"); err != nil {
		return cleanup(err)
	}
	if err := selectListEntry(m, idx); err != nil {
		return cleanup(fmt.Errorf("skill: Player PC: select bag item %#02x: %w", item, err))
	}

	// Non-key items (including every TM) ask for a quantity. Key items skip
	// that screen and transfer one immediately. Support both so the primitive
	// remains a real Player-PC operation rather than a TM-only special case.
	quantitySelected := false
	var mem state.Mem
	for spent := 0; spent < pcTransitionBudget; spent += talkSettle {
		state.Snapshot(m, &mem)
		_, bagNow := bagEntry(&mem, item)
		pcNow := pcItemQuantity(state.DecodePCItemStorage(&mem).Items, item)
		if bagNow == 0 && pcNow == pcBefore+bagBefore {
			break
		}
		if strings.Contains(strings.ToLower(state.ScreenText(&mem)), "no room left") {
			return cleanup(fmt.Errorf("%w", ErrPCItemStorageFull))
		}
		max := int(mem.U8(sym.MaxItemQuantity))
		cur := int(mem.U8(sym.ItemQuantity))
		if !quantitySelected && mem.U8(sym.MenuWatchedKeys) == watchListOrQty &&
			max == bagBefore && cur >= 1 && cur <= max {
			if err := selectBagQuantity(m, bagBefore); err != nil {
				return cleanup(fmt.Errorf("skill: Player PC: choose deposit quantity %d: %w", bagBefore, err))
			}
			quantitySelected = true
			continue
		}
		if mem.U8(sym.FontLoaded) != 0 && !state.MenuUp(&mem) {
			m.Tap(emu.A, 3, 7)
		}
		m.StepFrames(talkSettle)
	}

	state.Snapshot(m, &mem)
	_, bagAfter := bagEntry(&mem, item)
	pcAfter := pcItemQuantity(state.DecodePCItemStorage(&mem).Items, item)
	if bagAfter != 0 || pcAfter != pcBefore+bagBefore {
		return cleanup(fmt.Errorf("skill: Player PC: deposit postcondition failed for item %#02x: bag %d->%d PC %d->%d want %d",
			item, bagBefore, bagAfter, pcBefore, pcAfter, pcBefore+bagBefore))
	}
	if err := closePCToOverworld(m); err != nil {
		return err
	}
	return nil
}

// WithdrawPCItem withdraws qty units of an item from Player PC storage. The
// caller is responsible for ensuring the bag has a slot when this item is not
// already present.
func WithdrawPCItem(m *emu.Emu, romData []byte, policy MovePolicy, item uint8, qty int) error {
	if qty < 1 || qty > 99 {
		return fmt.Errorf("skill: Player PC: withdraw quantity %d out of range 1..99", qty)
	}
	if policy == nil {
		return fmt.Errorf("skill: Player PC: nil move policy")
	}
	var before state.Mem
	state.Snapshot(m, &before)
	storedBefore := state.DecodePCItemStorage(&before)
	pcBefore := pcItemQuantity(storedBefore.Items, item)
	if pcBefore < qty {
		return fmt.Errorf("%w: item %#02x stored x%d, need x%d", ErrPCItemNotStored, item, pcBefore, qty)
	}
	bagBefore := itemCount(&before, item)
	if bagBefore+qty > 99 {
		return fmt.Errorf("skill: Player PC: withdrawing item %#02x x%d would overflow bag stack x%d", item, qty, bagBefore)
	}
	if bagBefore == 0 && bagFreeSlots(&before) == 0 {
		return fmt.Errorf("skill: Player PC: no bag slot available for item %#02x", item)
	}

	if err := openPlayersPC(m, romData, policy); err != nil {
		return err
	}
	cleanup := func(err error) error {
		if closeErr := closePCToOverworld(m); closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}
	if err := SelectMenuItem(m, 0); err != nil { // WITHDRAW ITEM
		return cleanup(fmt.Errorf("skill: Player PC: select WITHDRAW ITEM: %w", err))
	}
	if err := pcAdvanceUntil(m, playersPCMenuUp, pcItemListUp, "stored-item list for withdraw"); err != nil {
		return cleanup(err)
	}
	idx := pcItemIndex(state.DecodePCItemStorage(&before).Items, item)
	if idx < 0 {
		return cleanup(fmt.Errorf("%w: item %#02x", ErrPCItemNotStored, item))
	}
	if err := selectListEntry(m, idx); err != nil {
		return cleanup(fmt.Errorf("skill: Player PC: select stored item %#02x: %w", item, err))
	}

	quantitySelected := false
	var mem state.Mem
	for spent := 0; spent < pcTransitionBudget; spent += talkSettle {
		state.Snapshot(m, &mem)
		bagNow := itemCount(&mem, item)
		pcNow := pcItemQuantity(state.DecodePCItemStorage(&mem).Items, item)
		if bagNow == bagBefore+qty && pcNow == pcBefore-qty {
			break
		}
		if strings.Contains(strings.ToLower(state.ScreenText(&mem)), "can't carry any more") {
			return cleanup(fmt.Errorf("skill: Player PC: bag cannot carry item %#02x", item))
		}
		max := int(mem.U8(sym.MaxItemQuantity))
		cur := int(mem.U8(sym.ItemQuantity))
		if !quantitySelected && mem.U8(sym.MenuWatchedKeys) == watchListOrQty &&
			max == pcBefore && cur >= 1 && cur <= max {
			if err := setQuantity(m, qty); err != nil {
				return cleanup(fmt.Errorf("skill: Player PC: choose withdraw quantity %d: %w", qty, err))
			}
			m.Tap(emu.A, 3, 7)
			quantitySelected = true
			continue
		}
		if mem.U8(sym.FontLoaded) != 0 && !state.MenuUp(&mem) {
			m.Tap(emu.A, 3, 7)
		}
		m.StepFrames(talkSettle)
	}

	state.Snapshot(m, &mem)
	bagAfter := itemCount(&mem, item)
	pcAfter := pcItemQuantity(state.DecodePCItemStorage(&mem).Items, item)
	if bagAfter != bagBefore+qty || pcAfter != pcBefore-qty {
		return cleanup(fmt.Errorf("skill: Player PC: withdraw postcondition failed for item %#02x: bag %d->%d want %d PC %d->%d want %d",
			item, bagBefore, bagAfter, bagBefore+qty, pcBefore, pcAfter, pcBefore-qty))
	}
	if err := closePCToOverworld(m); err != nil {
		return err
	}
	return nil
}
