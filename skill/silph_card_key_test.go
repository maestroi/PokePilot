package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestSilphCardKeySemanticHandoff(t *testing.T) {
	var mem state.Mem
	if SilphCardKeyReady(&mem) {
		t.Fatal("Card Key phase is ready before Saffron gate opens")
	}
	if SilphCardKeyOwned(&mem) {
		t.Fatal("empty bag reports Card Key owned")
	}

	// wStatusFlags1 bit 6 is BIT_GAVE_SAFFRON_GUARDS_DRINK; StoryFacts is
	// the authoritative decoder used by both gate and Card Key phases.
	mem[sym.StatusFlags1] |= 1 << 6
	if !SilphCardKeyReady(&mem) {
		t.Fatal("open Saffron gate did not make Card Key phase ready")
	}
	if SilphCardKeyOwned(&mem) {
		t.Fatal("gate opening alone reports Card Key owned")
	}

	putBag(&mem, state.BagItem{ID: silphCardKeyItem, Quantity: 1})
	if !SilphCardKeyOwned(&mem) {
		t.Fatal("Card Key bag entry did not satisfy semantic postcondition")
	}
}

func TestSilphCardKeyLandingIsNotPickupTile(t *testing.T) {
	if silphCardKeyLandingX == silphCardKeyX && silphCardKeyLandingY == silphCardKeyY {
		t.Fatal("cross-map navigation target must be a standing tile, not the Card Key object tile")
	}
	if silphCo1FMap == silphCo5FMap {
		t.Fatal("Silph floor map ids unexpectedly collapse")
	}
}
