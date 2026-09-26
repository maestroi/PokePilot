package agent_test

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestExecuteBuyApproachesMartClerk covers farm #1964/#1961/#1958. An
// ordinary KindBuy can be selected while the player is merely inside a Mart;
// skill.Buy itself intentionally assumes the clerk interaction boundary has
// already been reached. Turn away from the Viridian clerk first so the old
// executor would press A into empty floor and end in shop_menu_timeout.
func TestExecuteBuyApproachesMartClerk(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")

	var before state.Mem
	state.Snapshot(m, &before)
	bagBefore := redShopItemQuantity(&before, skill.ItemAntidote)
	moneyBefore := int(state.DecodeInventory(&before).Money)

	// The fixture starts at (2,5) facing the counter. Facing south either
	// turns in place or steps onto the open floor, but in both cases it
	// deliberately breaks Buy's clerk-facing precondition.
	if err := skill.Face(m, 2, 6); err != nil {
		t.Fatalf("turn away from mart clerk: %v", err)
	}

	o := agent.Objective{Kind: agent.KindBuy, Item: agent.ItemID("antidote"), Qty: 1}
	got, err := agent.Execute(m, m.ROM(), o)
	if err != nil {
		t.Fatalf("Execute %s from a non-clerk-facing Mart position: %v", o, err)
	}
	if got.Outcome != agent.OutcomeCompleted {
		t.Fatalf("buy Outcome = %q, want completed", got.Outcome)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if got := redShopItemQuantity(&after, skill.ItemAntidote); got != bagBefore+1 {
		t.Fatalf("ANTIDOTE count = %d, want %d", got, bagBefore+1)
	}
	if got := int(state.DecodeInventory(&after).Money); got != moneyBefore-100 {
		t.Fatalf("money = %d, want %d", got, moneyBefore-100)
	}
	if !state.Controllable(&after) {
		t.Fatal("player is not controllable after the purchase")
	}
}

func redShopItemQuantity(mem *state.Mem, id uint8) int {
	for _, item := range state.DecodeInventory(mem).Items {
		if item.ID == id {
			return int(item.Quantity)
		}
	}
	return 0
}
