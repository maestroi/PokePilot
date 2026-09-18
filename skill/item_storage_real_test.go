package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

func storedItemQty(mem *state.Mem, item uint8) int {
	for _, it := range state.DecodePCItemStorage(mem).Items {
		if it.ID == item {
			return int(it.Quantity)
		}
	}
	return 0
}

// TestPlayerPCItemRoundTrip is the emulator proof for the Player PC item
// storage controller. It buys a real ANTIDOTE, deposits the whole stack, then
// withdraws it again and verifies both inventories at each boundary.
func TestPlayerPCItemRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("emulator journey test")
	}
	m := fixture.Load(t, "viridian_mart")
	if err := skill.Buy(m, skill.ItemAntidote, 1); err != nil {
		t.Fatalf("buy setup item: %v", err)
	}
	policy := skill.StatAwareMove(m.ROM())

	if err := skill.DepositBagStack(m, m.ROM(), policy, skill.ItemAntidote); err != nil {
		t.Fatalf("deposit ANTIDOTE: %v", err)
	}
	var deposited state.Mem
	state.Snapshot(m, &deposited)
	if got := storedItemQty(&deposited, skill.ItemAntidote); got != 1 {
		t.Fatalf("stored ANTIDOTE = %d, want 1", got)
	}
	if got := state.DecodeInventory(&deposited).Items; func() bool {
		for _, it := range got {
			if it.ID == skill.ItemAntidote {
				return true
			}
		}
		return false
	}() {
		t.Fatal("ANTIDOTE remained in bag after deposit")
	}

	if err := skill.WithdrawPCItem(m, m.ROM(), policy, skill.ItemAntidote, 1); err != nil {
		t.Fatalf("withdraw ANTIDOTE: %v", err)
	}
	var withdrawn state.Mem
	state.Snapshot(m, &withdrawn)
	if got := storedItemQty(&withdrawn, skill.ItemAntidote); got != 0 {
		t.Fatalf("stored ANTIDOTE after withdraw = %d, want 0", got)
	}
	bagQty := 0
	for _, it := range state.DecodeInventory(&withdrawn).Items {
		if it.ID == skill.ItemAntidote {
			bagQty = int(it.Quantity)
		}
	}
	if bagQty != 1 {
		t.Fatalf("bag ANTIDOTE after withdraw = %d, want 1", bagQty)
	}
	if !state.Controllable(&withdrawn) {
		t.Fatal("player not controllable after Player PC round trip")
	}
}
