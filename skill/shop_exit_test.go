package skill_test

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestBuyUnstockedLeavesTheOverworld: the Viridian Mart does not sell
// POTION. Refusing an unstocked item used to return from INSIDE the item
// list, leaving it on screen for every later objective to die on.
func TestBuyUnstockedLeavesTheOverworld(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")

	// POTION is $14 (pokered/constants/item_constants.asm). The Viridian
	// Mart's shelf is POKe BALL, ANTIDOTE, PARLYZ HEAL, BURN HEAL.
	const itemPotion = 0x14
	err := skill.Buy(m, itemPotion, 3)
	if !errors.Is(err, skill.ErrNotInStock) {
		t.Fatalf("Buy = %v, want ErrNotInStock", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !state.Controllable(&after) {
		t.Fatalf("not controllable after an unstocked refusal: %q", state.ScreenText(&after))
	}
	if state.MenuUp(&after) {
		t.Fatalf("still inside a shop menu after an unstocked refusal: %q", state.ScreenText(&after))
	}
}

// TestBuyUnaffordableLeavesTheOverworld: refusing a purchase must leave the
// player standing in the mart, not parked inside its menus. The old backout
// stopped somewhere in the shop and reported ErrCantAfford anyway, which
// agent.Execute reads as a benign outcome — so the round was recorded DONE
// and every objective after it died on a screen nothing would close.
func TestBuyUnaffordableLeavesTheOverworld(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")

	var before state.Mem
	state.Snapshot(m, &before)
	moneyBefore := int(state.DecodeInventory(&before).Money)

	// 99 ANTIDOTEs is 9900: far beyond the fixture's purse.
	err := skill.Buy(m, skill.ItemAntidote, 99)
	if !errors.Is(err, skill.ErrCantAfford) {
		t.Fatalf("Buy = %v, want ErrCantAfford", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !state.Controllable(&after) {
		t.Fatalf("not controllable after a refused purchase: %q", state.ScreenText(&after))
	}
	if state.MenuUp(&after) {
		t.Fatalf("still inside a shop menu after a refused purchase: %q", state.ScreenText(&after))
	}
	if got := int(state.DecodeInventory(&after).Money); got != moneyBefore {
		t.Fatalf("money = %d, want %d unchanged by a refused purchase", got, moneyBefore)
	}
}
