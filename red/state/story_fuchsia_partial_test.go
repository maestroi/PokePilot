package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeStoryFactsPreservesPartialFuchsiaState(t *testing.T) {
	var mem Mem
	mem[sym.ObtainedBadges] = 1 << uint8(BadgeSoul)
	inv := InventoryState{Items: []BagItem{{ID: hm04ItemID, Quantity: 1}}}

	facts := DecodeStoryFacts(&mem, inv)
	if !facts.HM04Acquired {
		t.Fatal("HM04 should be visible independently")
	}
	if facts.HM03Acquired {
		t.Fatal("HM03 should remain false when Surf is missing")
	}
	if facts.FuchsiaProgressionComplete {
		t.Fatal("Fuchsia progression must remain incomplete until Soul + HM03 + HM04 are all present")
	}
}
