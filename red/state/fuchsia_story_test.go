package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeStoryFactsDecomposesFuchsiaProgression(t *testing.T) {
	var mem Mem
	mem[sym.ObtainedBadges] = 1 << uint8(BadgeSoul)
	inv := InventoryState{Items: []BagItem{{ID: hm04ItemID, Quantity: 1}}}

	facts := DecodeStoryFacts(&mem, inv)
	if !facts.SoulBadgeOwned {
		t.Fatal("Soul Badge was not projected independently")
	}
	if facts.HM03Acquired {
		t.Fatal("HM03 was projected when it is absent")
	}
	if !facts.HM04Acquired {
		t.Fatal("HM04 was not projected independently")
	}
	if facts.FuchsiaProgressionComplete {
		t.Fatal("partial Soul+HM04 state incorrectly marked Fuchsia complete")
	}

	inv.Items = append(inv.Items, BagItem{ID: hm03ItemID, Quantity: 1})
	facts = DecodeStoryFacts(&mem, inv)
	if !facts.HM03Acquired || !facts.HM04Acquired || !facts.SoulBadgeOwned {
		t.Fatalf("completed Fuchsia components not all projected: %+v", facts)
	}
	if !facts.FuchsiaProgressionComplete {
		t.Fatal("Soul+HM03+HM04 did not mark Fuchsia progression complete")
	}
}
