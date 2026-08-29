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

func countItem(mem *state.Mem, id uint8) int {
	for _, it := range state.DecodeInventory(mem).Items {
		if it.ID == id {
			return int(it.Quantity)
		}
	}
	return 0
}
