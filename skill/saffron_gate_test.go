package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestGuardDrinkInBag also pins the prerequisite of the guard-drink step to
// the drink itself. The gate once required the Soul Badge, Surf and Strength
// (the completed Fuchsia slice) before it would run. Saffron is the only
// corridor between Celadon and Vermilion, so that requirement made the Thunder
// Badge unreachable and deadlocked every run that had crossed into Celadon
// with two badges (run-jxh8lk19wv6on, run-1biaubd9xooqm). A ¥200 FRESH WATER
// from Celadon's roof is purchasable with no badge, HM or story fact, so a
// fresh save carrying only money is enough.
func TestGuardDrinkInBag(t *testing.T) {
	for _, tc := range []struct {
		name string
		item uint8
	}{
		{name: "fresh water", item: freshWaterItem},
		{name: "soda pop", item: sodaPopItem},
		{name: "lemonade", item: lemonadeItem},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mem state.Mem
			putBag(&mem, state.BagItem{ID: tc.item, Quantity: 1})
			got, ok := guardDrinkInBag(&mem)
			if !ok || got != tc.item {
				t.Fatalf("guardDrinkInBag = %#02x,%v; want %#02x,true", got, ok, tc.item)
			}
		})
	}

	var empty state.Mem
	putBag(&empty, state.BagItem{ID: 0x14, Quantity: 5}) // POTION is not a guard drink.
	if got, ok := guardDrinkInBag(&empty); ok {
		t.Fatalf("guardDrinkInBag accepted non-drink %#02x", got)
	}
}

func TestSaffronGateOpenUsesSemanticStoryFact(t *testing.T) {
	var mem state.Mem
	if SaffronGateOpen(&mem) {
		t.Fatal("fresh state reports Saffron gate open")
	}
	mem[sym.StatusFlags1] |= 1 << 6 // BIT_GAVE_SAFFRON_GUARDS_DRINK
	if !SaffronGateOpen(&mem) {
		t.Fatal("Saffron guard-drink flag did not satisfy semantic postcondition")
	}
}
