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
		// Refusing the purchase is not enough: the item list is UP, and
		// returning from here left it up. Every later objective then
		// refuses to start on a screen nothing closes. MEASURED
		// 2026-08-31: the Viridian Mart stocks POKe BALL, ANTIDOTE, PARLYZ
		// HEAL and BURN HEAL — no POTION — so "buy 3 POTION" returned
		// ErrNotInStock from inside the shop and killed the run four
		// rounds later. Same rule as the affordability refusal below: leave
		// the world where it was found, and say so loudly when you cannot.
		if err := exitToOverworld(m); err != nil {
			return fmt.Errorf("skill: Buy: item %#02x is not stocked and the shop did not close: %w", item, err)
		}
		return fmt.Errorf("skill: Buy: %w: item %#02x", ErrNotInStock, item)
	}

	// 4. Select the item; the choose-quantity box opens. Capture the stale
	// hMoney first so the box is detected by the price changing (there is no
	// other RAM marker that distinguishes it from the item list).
	hBefore := bcdMoney(&mem)
	if err := selectListEntry(m, pos); err != nil {
		// Same rule as the ErrNotInStock backout above: a cursor that never
		// reached its target leaves the item list up, and returning here
		// without closing it wedges every later objective on the same
		// unanswered shop menu.
		if exitErr := exitToOverworld(m); exitErr != nil {
			return fmt.Errorf("skill: Buy: select item %#02x: %v (shop did not close: %w)", item, err, exitErr)
		}
		return fmt.Errorf("skill: Buy: select item %#02x: %w", item, err)
	}
	qtyUp := func(mm *state.Mem) bool { return bcdMoney(mm) > 0 && bcdMoney(mm) != hBefore }
	if err := martWait(m, qtyUp, "the choose-quantity box"); err != nil {
		// Same rule as the ErrNotInStock backout above: returning here
		// leaves the item list (or whatever selectListEntry's A landed on)
		// up, and every later objective then fails on a shop menu nothing
		// closed. MEASURED 2026-09-02: "buy 3 POKEBALL" timed out here and
		// parked the run on "MONEY BUY... Is there anything else I can
		// do?" for the rest of the run.
		if exitErr := exitToOverworld(m); exitErr != nil {
			return fmt.Errorf("%w (shop did not close: %v)", err, exitErr)
		}
		return err
	}

	// 5. Set the quantity; hMoney now holds the total price for it.
	if err := setQuantity(m, qty); err != nil {
		if exitErr := exitToOverworld(m); exitErr != nil {
			return fmt.Errorf("%w (shop did not close: %v)", err, exitErr)
		}
		return err
	}
	state.Snapshot(m, &mem)
	total := bcdMoney(&mem)

	// 6. Affordability: a typed refusal, not a silent no-op or a hang.
	if moneyBefore < total {
		if err := backOutOfShop(m); err != nil {
			// NOT wrapped in ErrCantAfford. A caller that sees ErrCantAfford
			// is told "the game said no, the world is fine, pick something
			// else" — agent.Execute treats it as a benign outcome and
			// reports the round DONE. A backout that failed leaves the shop
			// menus on screen, which is the opposite of fine: every later
			// objective refuses to start on it. MEASURED 2026-08-31: "buy 3
			// POTION -> done" with 793 in hand against a 900 total, and the
			// run dead four rounds later on an item list nobody closed.
			return fmt.Errorf("skill: Buy: cannot afford %d (have %d) and the shop did not close: %w", total, moneyBefore, err)
		}
		return fmt.Errorf("skill: Buy: %w: have %d, need %d", ErrCantAfford, moneyBefore, total)
	}

	// 7. Confirm the quantity; the "That will be ¥X. OK?" box closes to a
	// two-option prompt.
	m.Tap(emu.A, 3, 7)
	if err := martAdvance(m, twoOptionUp, "the purchase-confirmation prompt"); err != nil {
		if exitErr := exitToOverworld(m); exitErr != nil {
			return fmt.Errorf("%w (shop did not close: %v)", err, exitErr)
		}
		return err
	}

	// 8. Answer YES (menu index 0). The trap: never a bare Tap(A) on the box.
	if err := SelectMenuItem(m, 0); err != nil {
		if exitErr := exitToOverworld(m); exitErr != nil {
			return fmt.Errorf("skill: Buy: answer YES: %v (shop did not close: %w)", err, exitErr)
		}
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

// listPosition is the item list's entry under the cursor. The mart's list
// menu scrolls exactly like the battle bag list (skill/bag.go's
// bagPosition): past the visible window wCurrentMenuItem stops moving and
// wListScrollOffset takes over, so the true index is their sum, not
// wCurrentMenuItem alone. wMaxMenuItem cannot substitute for either — it is
// a 1/2 window-size sentinel on list menus (DisplayListMenuID), not the
// entry count.
func listPosition(mm *state.Mem) int {
	return int(mm.U8(sym.ListScrollOffset)) + int(mm.U8(sym.CurrentMenuItem))
}

// selectListEntry drives the list-menu cursor to a 0-based position and
// presses A. Step-and-verify against listPosition, the same pattern
// selectBagEntry uses and for the same reason: a press count assumes
// wCurrentMenuItem alone tracks the selection, which is only true inside
// the visible window. MEASURED 2026-09-02: "buy 3 BURN HEAL" (index 3, the
// stock's 4th and last entry) never reached it — the mart's list window is
// 3 rows, so selecting it scrolls and wCurrentMenuItem pins at 2 while
// wListScrollOffset climbs to 1; a loop watching only wCurrentMenuItem
// never sees a match and burns its budget stuck on "2".
func selectListEntry(m *emu.Emu, index int) error {
	const stuckLimit = 8
	stuck := 0
	// The list needs one settle window before it will accept input at all:
	// martAdvance(itemListUp) returns the instant the list appears, which
	// can be mid-render. Every OTHER path through the loop below gets that
	// settle for free (a Down/Up tap is always followed by one before the
	// next read), but the entry already under the cursor takes zero loop
	// iterations and used to go straight to a bare Tap(A) — the confirming
	// press landed before the list was ready to see it and nothing
	// happened. MEASURED 2026-09-02: "buy 3 POKEBALL" right after buying
	// something else, with POKe BALL (index 0) already under the cursor,
	// silently dropped the selection and the choose-quantity box never
	// opened. Settling here first closes the gap for every entry, not only
	// the one already selected.
	m.StepFrames(talkSettle)
	var mem state.Mem
	state.Snapshot(m, &mem)
	for pos := listPosition(&mem); pos != index; pos = listPosition(&mem) {
		dir := emu.Down
		if index < pos {
			dir = emu.Up
		}
		m.Tap(dir, 3, 7)
		m.StepFrames(talkSettle)
		state.Snapshot(m, &mem)
		if listPosition(&mem) == pos {
			stuck++
			if stuck >= stuckLimit {
				return fmt.Errorf("cursor stuck at list entry %d, wanted %d, %d consecutive taps without movement", pos, index, stuck)
			}
		} else {
			stuck = 0
		}
	}
	m.Tap(emu.A, 3, 7)
	m.StepFrames(talkSettle)
	return nil
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

// backOutOfShop leaves the shop from the choose-quantity box (used to refuse
// an unaffordable purchase).
//
// It is exitToOverworld and nothing else. The old version choreographed the
// exit — B to the item list, settle for a redraw with no RAM marker, then
// leaveFromItemList's B / wait-for-BUY-SELL-QUIT / B — and every step of
// that assumed the screen it expected was the screen that was there. One
// stale wMenuWatchedKeys and the sequence pressed its buttons at the wrong
// menus and stopped somewhere inside the shop. exitToOverworld does not
// assume: it looks at what is on screen and presses the button that closes
// THAT, until the player is controllable and stays controllable.
func backOutOfShop(m *emu.Emu) error {
	return exitToOverworld(m)
}

// leaveFromItemList exits from the item list: B -> the "anything else" text
// auto-advances to the BUY/SELL/QUIT menu -> B quits -> then back to the
// overworld, whatever the shop still has on screen.
func leaveFromItemList(m *emu.Emu) error {
	m.Tap(emu.B, 3, 7)
	if err := martWait(m, buySellQuitUp, "the BUY/SELL/QUIT menu after leaving the item list"); err != nil {
		return err
	}
	m.Tap(emu.B, 3, 7)
	return exitToOverworld(m)
}

// exitToOverworld backs out of whatever is still on screen until the player
// is controllable, pressing the button that CLOSES what is up: B on a menu,
// A on ordinary text.
//
// It replaces a martAdvance(state.Controllable) that pressed A at every
// screen. Two things were wrong with that. A on a menu SELECTS, so the exit
// path could re-enter the shop it was leaving; and the predicate was checked
// on single snapshots, so the one-frame gap between one menu closing and the
// next drawing reads as "controllable" — Buy then returned SUCCESS with the
// item list still up, and every objective after it failed on a screen
// nothing would clear. MEASURED 2026-08-31: "buy 3 POTION -> done" followed
// immediately by "MONEY BUY Y793 ... POKe BALL ... Take your time.".
//
// So the exit confirms: controllable, then still controllable one settle
// later. A screen that redraws in that window is not an exit.
func exitToOverworld(m *emu.Emu) error {
	var mem state.Mem
	for i := 0; i < martWaitBudget; i++ {
		state.Snapshot(m, &mem)
		switch {
		case state.Controllable(&mem):
			m.StepFrames(talkSettle)
			state.Snapshot(m, &mem)
			if state.Controllable(&mem) {
				return nil
			}
		case state.MenuUp(&mem):
			m.Tap(emu.B, 3, 7) // B backs out of every Gen 1 menu
			m.StepFrames(talkSettle)
		case mem.U8(sym.FontLoaded) != 0:
			m.Tap(emu.A, 3, 7) // ordinary text: page it closed
			m.StepFrames(talkSettle)
		default:
			m.StepFrames(talkSettle) // mid-transition; let it land
		}
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: Buy: still not controllable after leaving the shop (wFontLoaded=%#04x wJoyIgnore=%#04x menu=%t screen=%q)",
		mem.U8(sym.FontLoaded), mem.U8(sym.JoyIgnore), state.MenuUp(&mem), state.ScreenText(&mem))
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
