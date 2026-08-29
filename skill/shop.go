package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ItemAntidote is the bag item ID of ANTIDOTE (pokered/constants/
// item_constants.asm: `const ANTIDOTE ; $0b`), stocked by the Viridian Mart.
const ItemAntidote = 0x0b

// ErrCantAfford is returned by Buy when the player's money is below the
// purchase total. It is a typed result, not a silent no-op: Buy backs out of
// the clerk's menus and leaves the player controllable before returning it.
var ErrCantAfford = errors.New("skill: not enough money for the purchase")

// ErrNotInStock is returned by Buy when the clerk does not sell the requested
// item. The stock is read from wItemList (the ROM's mart table), never a
// hardcoded price list.
var ErrNotInStock = errors.New("skill: the clerk does not stock the requested item")

// wMenuWatchedKeys values that identify the mart's menus. The BUY/SELL/QUIT
// menu and the two-option prompt watch only A|B (3); the priced item list and
// the choose-quantity box watch A|B|SELECT (7). These are the only signals that
// tell the four shop screens apart, since wMaxMenuItem is a 1/2 sentinel on the
// lists (DisplayListMenuID) and stale otherwise.
const (
	watchBuySellQuit = 3
	watchListOrQty   = 7
)

// Budgets for the mart's short text/menu transitions. Each advanceUntil
// iteration is either an A-tap plus talkSettle or a single frame; each
// martWait iteration is one talkSettle block.
const (
	martAdvanceBudget = 500
	martWaitBudget    = 60
	martQtyBudget     = 120
	backOutRedraw     = 8 // talkSettle blocks for the item list to redraw after B on the quantity box
)

// Buy purchases qty of item from a mart clerk. It talks to the clerk, selects
// BUY, picks the item, sets the quantity, confirms (answering YES to the
// two-option prompt), and exits the menus, leaving the player controllable.
// On success it asserts the bag count rose by qty AND the money fell by the
// total. If the player cannot afford it, it backs out cleanly and returns
// ErrCantAfford; if the clerk does not stock the item, ErrNotInStock.
func Buy(m *emu.Emu, item uint8, qty int) error {
	if qty < 1 || qty > 99 {
		return fmt.Errorf("skill: Buy: quantity %d out of range 1..99", qty)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: Buy: not controllable (wFontLoaded=%#04x wJoyIgnore=%#04x)",
			mem.U8(sym.FontLoaded), mem.U8(sym.JoyIgnore))
	}
	before := state.DecodeInventory(&mem)
	moneyBefore := int(before.Money)
	bagBefore := bagCount(before.Items, item)

	// 1. Open the shop: A on the clerk auto-advances the greeting to the
	// BUY/SELL/QUIT menu (wMenuWatchedKeys == A|B).
	m.Tap(emu.A, 3, 7)
	if err := martAdvance(m, buySellQuitUp, "the BUY/SELL/QUIT menu"); err != nil {
		return err
	}

	// 2. Select BUY (the cursor starts on BUY).
	if err := SelectMenuItem(m, 0); err != nil {
		return fmt.Errorf("skill: Buy: select BUY: %w", err)
	}

	// 3. Advance to the priced item list ("Take your time." then the list).
	if err := martAdvance(m, itemListUp, "the item list"); err != nil {
		return err
	}
	state.Snapshot(m, &mem)
	pos, ok := martItemPosition(&mem, item)
	if !ok {
		return fmt.Errorf("skill: Buy: %w: item %#02x", ErrNotInStock, item)
	}

	// 4. Select the item; the choose-quantity box opens. Capture the stale
	// hMoney first so the box is detected by the price changing (there is no
	// other RAM marker that distinguishes it from the item list).
	hBefore := bcdMoney(&mem)
	if err := selectListEntry(m, pos); err != nil {
		return fmt.Errorf("skill: Buy: select item %#02x: %w", item, err)
	}
	qtyUp := func(mm *state.Mem) bool { return bcdMoney(mm) > 0 && bcdMoney(mm) != hBefore }
	if err := martWait(m, qtyUp, "the choose-quantity box"); err != nil {
		return err
	}

	// 5. Set the quantity; hMoney now holds the total price for it.
	if err := setQuantity(m, qty); err != nil {
		return err
	}
	state.Snapshot(m, &mem)
	total := bcdMoney(&mem)

	// 6. Affordability: a typed refusal, not a silent no-op or a hang.
	if moneyBefore < total {
		if err := backOutOfShop(m); err != nil {
			return fmt.Errorf("skill: Buy: %w (backing out: %v)", ErrCantAfford, err)
		}
		return fmt.Errorf("skill: Buy: %w: have %d, need %d", ErrCantAfford, moneyBefore, total)
	}

	// 7. Confirm the quantity; the "That will be ¥X. OK?" box closes to a
	// two-option prompt.
	m.Tap(emu.A, 3, 7)
	if err := martAdvance(m, twoOptionUp, "the purchase-confirmation prompt"); err != nil {
		return err
	}

	// 8. Answer YES (menu index 0). The trap: never a bare Tap(A) on the box.
	if err := SelectMenuItem(m, 0); err != nil {
		return fmt.Errorf("skill: Buy: answer YES: %w", err)
	}

	// 9. The purchase runs; leave the shop and wait until controllable.
	if err := exitShop(m); err != nil {
		return err
	}

	// 10. Postconditions: bag rose by qty AND money fell by the total.
	state.Snapshot(m, &mem)
	after := state.DecodeInventory(&mem)
	bagAfter := bagCount(after.Items, item)
	if bagAfter != bagBefore+qty {
		return fmt.Errorf("skill: Buy: bag count for item %#02x = %d, want %d (before %d + %d)",
			item, bagAfter, bagBefore+qty, bagBefore, qty)
	}
	if int(after.Money) != moneyBefore-total {
		return fmt.Errorf("skill: Buy: money = %d, want %d (before %d - total %d)",
			after.Money, moneyBefore-total, moneyBefore, total)
	}
	return nil
}

// buySellQuitUp reports that the BUY/SELL/QUIT menu is up. It is false while
// controllable (wFontLoaded == 0) and while the item list / quantity box are up
// (wMenuWatchedKeys == A|B|SELECT), so it cannot fire on a stale value.
func buySellQuitUp(mm *state.Mem) bool {
	return mm.U8(sym.FontLoaded) != 0 && mm.U8(sym.MenuWatchedKeys) == watchBuySellQuit
}

// itemListUp reports that the priced item list is up. It is only used at points
// in the flow where the choose-quantity box is not (yet) up, so A|B|SELECT here
// means the list.
func itemListUp(mm *state.Mem) bool {
	return mm.U8(sym.MenuWatchedKeys) == watchListOrQty
}

// twoOptionUp reports that a two-option prompt (the YES/NO confirmation) is up.
func twoOptionUp(mm *state.Mem) bool {
	return state.DecodeTwoOptionMenu(mm) != nil
}

// martAdvance steps frames, pressing A while a text box is up, until pred holds.
// It returns a diagnostic error if pred never holds within the budget. Because
// pred is checked before every A-tap, it never presses A on the menu it is
// waiting for.
func martAdvance(m *emu.Emu, pred func(*state.Mem) bool, what string) error {
	mem := advanceUntil(m, martAdvanceBudget, pred)
	if !pred(&mem) {
		return martTimeout(what, &mem)
	}
	return nil
}

// martWait steps frames with NO input until pred holds, for transitions that
// advance on their own (the "anything else" text auto-advancing to the
// BUY/SELL/QUIT menu, the quantity box computing its price).
func martWait(m *emu.Emu, pred func(*state.Mem) bool, what string) error {
	var mem state.Mem
	for i := 0; i < martWaitBudget; i++ {
		state.Snapshot(m, &mem)
		if pred(&mem) {
			return nil
		}
		m.StepFrames(talkSettle)
	}
	state.Snapshot(m, &mem)
	if !pred(&mem) {
		return martTimeout(what, &mem)
	}
	return nil
}

func martTimeout(what string, mem *state.Mem) error {
	return fmt.Errorf("skill: Buy: %s did not appear (wFontLoaded=%#04x wCurMenuItem=%d wMaxMenuItem=%d wItemQuantity=%d wMoney=%d)",
		what, mem.U8(sym.FontLoaded), mem.U8(sym.CurrentMenuItem), mem.U8(sym.MaxMenuItem),
		mem.U8(sym.ItemQuantity), bcdMoney(mem))
}

// selectListEntry drives the list-menu cursor to a 0-based position and presses
// A. The list menu's cursor is wCurrentMenuItem and its range is the list
// length, not wMaxMenuItem (which DisplayListMenuID clamps to a 1/2 sentinel),
// so SelectMenuItem cannot drive it.
func selectListEntry(m *emu.Emu, index int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if int(mem.U8(sym.CurrentMenuItem)) == index {
		m.Tap(emu.A, 3, 7)
		m.StepFrames(talkSettle)
		return nil
	}
	dir := emu.Down
	if index < int(mem.U8(sym.CurrentMenuItem)) {
		dir = emu.Up
	}
	for i := 0; i < 24; i++ {
		m.Tap(dir, 3, 7)
		m.StepFrames(talkSettle)
		state.Snapshot(m, &mem)
		if int(mem.U8(sym.CurrentMenuItem)) == index {
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
			return nil
		}
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("cursor did not reach list entry %d (wCurMenuItem=%d)", index, mem.U8(sym.CurrentMenuItem))
}

// setQuantity drives the choose-quantity selector to qty by tapping Up (it
// starts at 1 and only increments upward here), verifying wItemQuantity after
// each tap.
func setQuantity(m *emu.Emu, qty int) error {
	for i := 0; i < martQtyBudget; i++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		cur := int(mem.U8(sym.ItemQuantity))
		if cur == qty {
			return nil
		}
		if cur > qty {
			return fmt.Errorf("quantity overshot %d (wItemQuantity=%d)", qty, cur)
		}
		m.Tap(emu.Up, 3, 7)
		m.StepFrames(talkSettle)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	return fmt.Errorf("quantity did not reach %d (wItemQuantity=%d)", qty, mem.U8(sym.ItemQuantity))
}

// exitShop leaves the shop after a successful purchase: close "Here you are!"
// to the item list, then out through leaveFromItemList.
func exitShop(m *emu.Emu) error {
	if err := martAdvance(m, itemListUp, "the item list after the purchase"); err != nil {
		return err
	}
	return leaveFromItemList(m)
}

// backOutOfShop leaves the shop from the choose-quantity box (used to refuse an
// unaffordable purchase): B back to the item list, then out. There is no RAM
// marker for the quantity->list redraw (both watch A|B|SELECT and leave
// wMaxItemQuantity stale), so settle for the redraw before the next B.
func backOutOfShop(m *emu.Emu) error {
	m.Tap(emu.B, 3, 7)
	m.StepFrames(backOutRedraw * talkSettle)
	return leaveFromItemList(m)
}

// leaveFromItemList exits from the item list: B -> the "anything else" text
// auto-advances to the BUY/SELL/QUIT menu -> B quits -> wait for controllable
// (the farewell box needs an A).
func leaveFromItemList(m *emu.Emu) error {
	m.Tap(emu.B, 3, 7)
	if err := martWait(m, buySellQuitUp, "the BUY/SELL/QUIT menu after leaving the item list"); err != nil {
		return err
	}
	m.Tap(emu.B, 3, 7)
	return martAdvance(m, state.Controllable, "controllable after quitting the shop")
}

// martItemPosition returns the 0-based position of item in the clerk's stock
// (wItemList: a count byte then item ids, $ff-terminated). It reports ok=false
// if the clerk does not sell it.
func martItemPosition(mem *state.Mem, item uint8) (int, bool) {
	for i := 1; ; i++ {
		v := mem.U8(sym.ItemList + uint16(i))
		if v == 0xff {
			break
		}
		if v == item {
			return i - 1, true
		}
	}
	return 0, false
}

// bcdMoney decodes the 3-byte BCD value at addr (hMoney for the shop's total).
func bcdMoney(m *state.Mem) int {
	v := 0
	for _, b := range m.Slice(sym.Money, 3) {
		v = v*100 + int(b>>4)*10 + int(b&0x0F)
	}
	return v
}

// bagCount returns the quantity of id in the bag (0 if absent).
func bagCount(items []state.BagItem, id uint8) int {
	for _, it := range items {
		if it.ID == id {
			return int(it.Quantity)
		}
	}
	return 0
}
