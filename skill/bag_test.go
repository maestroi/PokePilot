package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// itemPotion is the bag item ID of POTION (pokered/constants/item_constants.asm:
// `const POTION ; $14`).
const itemPotion = 0x14

// travelToRoute1 moves the player to Route 1, whose tall grass throws wild
// PIDGEY — the only battle reachable from this fixture.
func travelToRoute1(t *testing.T, m *emu.Emu) {
	t.Helper()
	r1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal(`Place "route 1" not found`)
	}
	if _, err := skill.Travel(m, m.ROM(), r1, skill.StatAwareMove(m.ROM()), 20); err != nil {
		t.Fatalf("travel to Route 1: %v", err)
	}
}

// bagQty reports the quantity of item in the bag, or 0 when the bag holds no
// such entry.
func bagQty(t *testing.T, m *emu.Emu, item uint8) int {
	t.Helper()
	var mem state.Mem
	state.Snapshot(m, &mem)
	for _, it := range state.DecodeInventory(&mem).Items {
		if it.ID == item {
			return int(it.Quantity)
		}
	}
	return 0
}

// TestUseItemWildBattle is the S6-2 postcondition: from an open battle main
// menu, UseItem uses one bag item and the bag's count for it DROPS by one.
// The post_pokeballs fixture carries five POKE BALLS and nothing else
// usable, so the item used is a ball; whether the throw actually catches
// the wild PIDGEY is S6-3's concern — here only the bag is asserted, and
// the drop happens on a failed catch just as on a successful one.
func TestUseItemWildBattle(t *testing.T) {
	m := fixture.Load(t, "post_pokeballs")

	if q := bagQty(t, m, skill.ItemPokeBall); q != 5 {
		t.Fatalf("fixture precondition: expected five POKE BALLS in the bag, got %d", q)
	}

	travelToRoute1(t, m)
	if err := skill.EnterWildBattle(m, 3); err != nil {
		t.Fatalf("enter wild battle: %v", err)
	}
	if err := skill.UseItem(m, skill.ItemPokeBall); err != nil {
		t.Fatalf("use POKE BALL: %v", err)
	}

	if q := bagQty(t, m, skill.ItemPokeBall); q != 4 {
		t.Fatalf("postcondition: bag count for POKE BALL did not drop from 5 to 4, got %d", q)
	}
}

// TestUseItemNotInBag checks the negative path: an item the bag does not
// hold is refused without touching the game.
func TestUseItemNotInBag(t *testing.T) {
	m := fixture.Load(t, "post_pokeballs")

	travelToRoute1(t, m)
	if err := skill.EnterWildBattle(m, 3); err != nil {
		t.Fatalf("enter wild battle: %v", err)
	}
	err := skill.UseItem(m, itemPotion)
	if err == nil {
		t.Fatal("UseItem(POTION) with an empty-of-potions bag: want error, got nil")
	}
	if q := bagQty(t, m, skill.ItemPokeBall); q != 5 {
		t.Fatalf("refused use must not touch the bag: POKE BALL count is %d, want 5", q)
	}
}
