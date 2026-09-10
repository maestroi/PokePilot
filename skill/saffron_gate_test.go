package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

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

func TestSaffronGateReadyRequiresCompletedFuchsiaSlice(t *testing.T) {
	var mem state.Mem
	mem[sym.ObtainedBadges] |= 1 << 4 // Soul Badge
	putBag(&mem,
		state.BagItem{ID: hm03SurfItem, Quantity: 1},
		state.BagItem{ID: hm04StrengthItem, Quantity: 1},
	)
	if !SaffronGateReady(&mem) {
		t.Fatal("Soul Badge + Surf + Strength did not satisfy #33 handoff")
	}

	putBag(&mem, state.BagItem{ID: hm03SurfItem, Quantity: 1})
	if SaffronGateReady(&mem) {
		t.Fatal("missing Strength still reported Saffron gate ready")
	}
}
