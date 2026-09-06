package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	gen1BagCapacity       = 20
	bagQuantityMenuBudget = 300
	bagTossConfirmBudget  = 600
	bagTossSettleBudget   = 3000
)

// ErrNoSafeBagSpace reports that a full bag has no stack this skill is
// explicitly willing to discard. Unknown items, key/story items, TMs/HMs and
// scarce high-value resources are protected by omission rather than by trying
// to infer that they are safe at runtime.
var ErrNoSafeBagSpace = errors.New("skill: no safe expendable bag stack can be discarded")

// safeBagSacrificeUnitCost is intentionally a whitelist. Every listed item is
// a replenishable consumable in Red; anything absent is protected. In
// particular this excludes MASTER BALL, every key/story item, evolution
// stones, vitamins/RARE CANDY, PP recovery, every TM/HM, and unknown IDs.
// Values are the Pokemart buy prices from data/items/prices.asm and are used
// only to choose the least costly whole stack to sacrifice.
var safeBagSacrificeUnitCost = map[uint8]int{
	0x02: 1200, // ULTRA BALL
	0x03: 600,  // GREAT BALL
	0x04: 200,  // POKE BALL
	0x0B: 100,  // ANTIDOTE
	0x0C: 250,  // BURN HEAL
	0x0D: 250,  // ICE HEAL
	0x0E: 200,  // AWAKENING
	0x0F: 200,  // PARLYZ HEAL
	0x13: 700,  // SUPER POTION
	0x14: 300,  // POTION
	0x1D: 550,  // ESCAPE ROPE
	0x1E: 350,  // REPEL
	0x2E: 950,  // X ACCURACY
	0x34: 600,  // FULL HEAL
	0x37: 700,  // GUARD SPEC.
	0x38: 500,  // SUPER REPEL
	0x39: 700,  // MAX REPEL
	0x3A: 650,  // DIRE HIT
	0x3C: 200,  // FRESH WATER
	0x3D: 300,  // SODA POP
	0x3E: 350,  // LEMONADE
	0x41: 500,  // X ATTACK
	0x42: 550,  // X DEFEND
	0x43: 350,  // X SPEED
	0x44: 350,  // X SPECIAL
}

func bagFreeSlots(mem *state.Mem) int {
	free := gen1BagCapacity - len(state.DecodeInventory(mem).Items)
	if free < 0 {
		return 0
	}
	return free
}

// chooseSafeBagSacrifice returns the cheapest whole expendable stack. A bag
// slot is keyed by distinct item ID, so tossing only part of a stack cannot
// create capacity; the score is therefore unit price * quantity. Ties prefer
// the smaller stack, then the earlier bag entry for deterministic behavior.
func chooseSafeBagSacrifice(inv state.InventoryState) (int, state.BagItem, bool) {
	bestIndex := -1
	var best state.BagItem
	bestCost := 0
	for i, it := range inv.Items {
		unit, ok := safeBagSacrificeUnitCost[it.ID]
		if !ok || it.Quantity == 0 {
			continue
		}
		cost := unit * int(it.Quantity)
		if bestIndex < 0 || cost < bestCost ||
			(cost == bestCost && it.Quantity < best.Quantity) {
			bestIndex, best, bestCost = i, it, cost
		}
	}
	return bestIndex, best, bestIndex >= 0
}

// EnsureBagSpaceFor guarantees that receiving one unit of item will not need
// a missing bag slot. If item is already present, Gen I stacks it into the
// existing entry and no space is needed.
func EnsureBagSpaceFor(m *emu.Emu, item uint8) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if hasBagItem(&mem, item) {
		return nil
	}
	return EnsureBagFreeSlots(m, 1)
}

// EnsureBagFreeSlots guarantees at least minFree distinct-item slots in the
// bag. It may discard multiple explicitly safe stacks, one at a time, and
// re-reads RAM after each toss. If the bag contains only protected items it
// fails without mutating them.
func EnsureBagFreeSlots(m *emu.Emu, minFree int) error {
	if minFree < 0 || minFree > gen1BagCapacity {
		return fmt.Errorf("skill: EnsureBagFreeSlots: requested %d free slots, want 0..%d", minFree, gen1BagCapacity)
	}
	if minFree == 0 {
		return nil
	}

	for {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if !state.Controllable(&mem) {
			return fmt.Errorf("skill: EnsureBagFreeSlots: player not controllable on map %#04x at (%d,%d)",
				mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
		}
		if free := bagFreeSlots(&mem); free >= minFree {
			return nil
		}

		inv := state.DecodeInventory(&mem)
		idx, sacrifice, ok := chooseSafeBagSacrifice(inv)
		if !ok {
			return fmt.Errorf("%w: bag uses %d/%d slots and %d free slot(s) are required",
				ErrNoSafeBagSpace, len(inv.Items), gen1BagCapacity, minFree)
		}
		if err := tossBagStack(m, idx, sacrifice); err != nil {
			return fmt.Errorf("skill: EnsureBagFreeSlots: toss item %#02x x%d: %w",
				sacrifice.ID, sacrifice.Quantity, err)
		}
	}
}

func openOverworldBagList(m *emu.Emu, mem *state.Mem) error {
	wantMax, itemIndex := startMenuShape(mem)
	drawn := func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == wantMax
	}
	for attempt := 0; attempt < 5 && !drawn(m); attempt++ {
		m.Tap(emu.Start, 3, 7)
		_, _ = m.StepUntil(startMenuDrawBudget, drawn)
	}
	if !drawn(m) {
		return fmt.Errorf("start menu did not draw")
	}
	if err := SelectMenuItem(m, itemIndex); err != nil {
		return fmt.Errorf("select ITEM: %w", err)
	}
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == itemListMenuID
	}); err != nil {
		return fmt.Errorf("bag list did not open: %w", err)
	}
	return nil
}

// selectBagQuantity drives DisplayChooseQuantityMenu by its actual RAM
// counter. DOWN decrements and wraps 1 -> max, which makes selecting the whole
// stack cheap while remaining step-and-verify rather than relying on a press
// count.
func selectBagQuantity(m *emu.Emu, target int) error {
	if target < 1 || target > 99 {
		return fmt.Errorf("skill: selectBagQuantity: target %d out of range 1..99", target)
	}
	if _, err := m.StepUntil(bagQuantityMenuBudget, func(m *emu.Emu) bool {
		max := int(m.Peek8(sym.MaxItemQuantity))
		cur := int(m.Peek8(sym.ItemQuantity))
		return max == target && cur >= 1 && cur <= max
	}); err != nil {
		return fmt.Errorf("quantity menu did not initialize for stack size %d: max=%d current=%d: %w",
			target, m.Peek8(sym.MaxItemQuantity), m.Peek8(sym.ItemQuantity), err)
	}

	const stuckLimit = 5
	stuck := 0
	for cur := int(m.Peek8(sym.ItemQuantity)); cur != target; cur = int(m.Peek8(sym.ItemQuantity)) {
		m.Tap(emu.Down, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.ItemQuantity)) != cur
		}); err != nil {
			stuck++
			if stuck >= stuckLimit {
				return fmt.Errorf("skill: selectBagQuantity: quantity stuck at %d, wanted %d: %w", cur, target, ErrMenuStuck)
			}
		} else {
			stuck = 0
		}
	}
	m.Tap(emu.A, 3, 7)
	return nil
}

func tossBagStack(m *emu.Emu, idx int, item state.BagItem) error {
	if _, safe := safeBagSacrificeUnitCost[item.ID]; !safe {
		return fmt.Errorf("refusing to toss protected item %#02x", item.ID)
	}
	if item.Quantity == 0 {
		return fmt.Errorf("refusing to toss zero-quantity item %#02x", item.ID)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	liveIdx, liveQty := bagEntry(&mem, item.ID)
	if liveIdx != idx || liveQty != int(item.Quantity) {
		return fmt.Errorf("bag changed before toss: item %#02x expected entry %d x%d, now entry %d x%d",
			item.ID, idx, item.Quantity, liveIdx, liveQty)
	}
	if err := openOverworldBagList(m, &mem); err != nil {
		return err
	}
	if err := selectBagEntry(m, idx); err != nil {
		return err
	}
	if _, err := m.StepUntil(useTossBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return useTossPrompt(&mem) != nil
	}); err != nil {
		return fmt.Errorf("USE/TOSS prompt did not open: %w", err)
	}
	if err := selectTwoOption(m, 1); err != nil { // TOSS
		return fmt.Errorf("select TOSS: %w", err)
	}
	if err := selectBagQuantity(m, int(item.Quantity)); err != nil {
		return err
	}

	// TossItem asks one final YES/NO at a different screen position than the
	// USE/TOSS menu. Wait for that exact semantic shape, then choose YES.
	if _, err := m.StepUntil(bagTossConfirmBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return state.DecodeTwoOptionMenu(&mem) != nil && useTossPrompt(&mem) == nil
	}); err != nil {
		return fmt.Errorf("toss confirmation did not appear: %w", err)
	}
	if err := selectTwoOption(m, 0); err != nil { // YES
		return fmt.Errorf("confirm toss: %w", err)
	}

	removed := false
	start := m.FrameCount()
	for {
		state.Snapshot(m, &mem)
		_, qty := bagEntry(&mem, item.ID)
		if qty == 0 {
			removed = true
		}
		if removed && state.Controllable(&mem) && mem.U8(sym.FontLoaded) == 0 {
			if free := bagFreeSlots(&mem); free < 1 {
				return fmt.Errorf("item %#02x disappeared but no bag slot became free", item.ID)
			}
			return nil
		}
		if int(m.FrameCount()-start) > bagTossSettleBudget {
			return fmt.Errorf("toss did not settle: item %#02x remaining=%d removed=%v screen=%q",
				item.ID, qty, removed, state.ScreenText(&mem))
		}
		// After YES there are no further choices: B advances the result text,
		// closes the bag list, then closes START without selecting anything.
		m.Tap(emu.B, 3, 7)
	}
}
