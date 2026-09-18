package skill_test

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestBuy is S6-1: from the viridian_mart fixture (the player on the counter
// approach tile, facing the clerk), buy two ANTIDOTEs and return with the bag
// holding two more AND the money down by the total, the player controllable.
//
// The fixture arrives with an empty bag and ¥1587 (measured on the committed
// state). ANTIDOTE is ¥100 each at the Viridian Mart, so two cost ¥200: the
// postconditions below pin both sides of the trade — a purchase that credits
// the bag without debiting money, or vice versa, fails here.
func TestBuy(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")

	var before state.Mem
	state.Snapshot(m, &before)
	if !state.Controllable(&before) {
		t.Fatal("precondition: fixture is not controllable")
	}
	bagBefore := countItem(&before, skill.ItemAntidote)
	moneyBefore := int(state.DecodeInventory(&before).Money)

	if err := skill.Buy(m, skill.ItemAntidote, 2); err != nil {
		t.Fatalf("Buy: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	bagAfter := countItem(&after, skill.ItemAntidote)
	moneyAfter := int(state.DecodeInventory(&after).Money)

	if bagAfter != bagBefore+2 {
		t.Errorf("bag: ANTIDOTE = %d, want %d (before %d + 2)", bagAfter, bagBefore+2, bagBefore)
	}
	if moneyAfter != moneyBefore-200 {
		t.Errorf("money: %d, want %d (before %d - 200)", moneyAfter, moneyBefore-200, moneyBefore)
	}
	if !state.Controllable(&after) {
		t.Error("postcondition: player is not controllable after the purchase")
	}
}

// TestBuyCantAfford is S6-1's refusal half: a quantity the fixture's ¥1587
// cannot cover (99 ANTIDOTEs = ¥9900) must come back as ErrCantAfford, with
// the player backed out of the clerk's menus and controllable — not a silent
// no-op and not a hang.
func TestBuyCantAfford(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")

	err := skill.Buy(m, skill.ItemAntidote, 99)
	if !errors.Is(err, skill.ErrCantAfford) {
		t.Fatalf("Buy: err = %v, want ErrCantAfford", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !state.Controllable(&after) {
		t.Error("postcondition: player is not controllable after the refusal")
	}
	// The purchase must NOT have happened.
	if got := countItem(&after, skill.ItemAntidote); got != 0 {
		t.Errorf("bag: ANTIDOTE = %d after a refused buy, want 0", got)
	}
}


// TestSell drives the real SELL half of the mart controller. Buy two
// ANTIDOTEs first so the committed fixture needs no RAM mutation, then sell
// the whole stack. Red pays half the shop price, so ¥200 spent becomes ¥100
// recovered; the distinct bag slot must disappear and the player must be back
// on a controllable overworld boundary.
func TestSell(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")
	if err := skill.Buy(m, skill.ItemAntidote, 2); err != nil {
		t.Fatalf("Buy setup: %v", err)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	moneyBefore := int(state.DecodeInventory(&before).Money)
	if got := countItem(&before, skill.ItemAntidote); got != 2 {
		t.Fatalf("setup ANTIDOTE = %d, want 2", got)
	}

	if err := skill.Sell(m, skill.ItemAntidote, 2); err != nil {
		t.Fatalf("Sell: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if got := countItem(&after, skill.ItemAntidote); got != 0 {
		t.Errorf("bag: ANTIDOTE = %d after sale, want 0", got)
	}
	if got := int(state.DecodeInventory(&after).Money); got != moneyBefore+100 {
		t.Errorf("money = %d, want %d (before %d + 100)", got, moneyBefore+100, moneyBefore)
	}
	if !state.Controllable(&after) {
		t.Error("postcondition: player is not controllable after sale")
	}
}

// TestTalkStopsAtShopMenu is the triage regression for 081eb00e22e44d40: the
// Viridian Mart clerk opens a BUY/SELL/QUIT menu, not a text box. Talk pages
// ordinary dialogue; it must not walk that menu into a purchase. From the
// viridian_mart fixture (player on the counter, facing the clerk), Talk opens
// the shop and stops at the menu with ErrTalkMenu, leaving the surface up for
// the owning layer to back out.
func TestTalkStopsAtShopMenu(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")

	presses, err := skill.Talk(m)
	t.Logf("Talk: presses=%d err=%v", presses, err)
	var menuErr *skill.ErrTalkMenu
	if !errors.As(err, &menuErr) {
		t.Fatalf("Talk: err = %v, want ErrTalkMenu", err)
	}
}

func countItem(mem *state.Mem, id uint8) int {
	for _, it := range state.DecodeInventory(mem).Items {
		if it.ID == id {
			return int(it.Quantity)
		}
	}
	return 0
}
