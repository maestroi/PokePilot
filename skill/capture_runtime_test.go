package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestCaptureAcquisitionUsesPortableState(t *testing.T) {
	before := game.CaptureState{
		PartySpecies: []uint16{0x101},
		OwnedDex:     []uint16{1},
	}
	after := game.CaptureState{
		PartySpecies: []uint16{0x101, 0x222},
		OwnedDex:     []uint16{1, 25},
	}
	got, ok := captureAcquiredWantedState(before, after, []uint16{0x222}, []uint16{25})
	if !ok || got != 0x222 {
		t.Fatalf("acquired = %#04x ok=%v, want native species %#04x", got, ok, 0x222)
	}
}

func TestCaptureAcquisitionAcceptsBoxGrowthAndDexEvidence(t *testing.T) {
	before := game.CaptureState{ActiveBoxSpecies: []uint16{0x10}, OwnedDex: []uint16{7}}
	after := game.CaptureState{ActiveBoxSpecies: []uint16{0x10, 0x333}, OwnedDex: []uint16{7, 151}}
	if got, ok := captureAcquiredWantedState(before, after, []uint16{0x333}, []uint16{151}); !ok || got != 0x333 {
		t.Fatalf("box acquisition = %#04x ok=%v", got, ok)
	}

	before = game.CaptureState{OwnedDex: []uint16{7}}
	after = game.CaptureState{OwnedDex: []uint16{7, 151}}
	if got, ok := captureAcquiredWantedState(before, after, []uint16{0x444}, []uint16{151}); !ok || got != 0x444 {
		t.Fatalf("dex acquisition = %#04x ok=%v", got, ok)
	}
}

func TestOrdinaryCaptureBallUsesProfileOrderAndWideIDs(t *testing.T) {
	inventory := game.InventoryState{Items: []game.InventoryItem{
		{NativeItemID: 0x120, Quantity: 4},
		{NativeItemID: 0x350, Quantity: 2},
	}}
	order := []uint16{0x350, 0x120}
	if got, ok := ordinaryCaptureBall(inventory, order); !ok || got != 0x350 {
		t.Fatalf("ball = %#04x ok=%v, want %#04x", got, ok, 0x350)
	}
	if got := ordinaryCaptureBallCount(inventory, order); got != 6 {
		t.Fatalf("ball count = %d, want 6", got)
	}
}

func TestLegacyCaptureExecutionRejectsWideNativeIDs(t *testing.T) {
	if _, err := legacyItemID(0x350); err == nil {
		t.Fatal("wide native item id unexpectedly truncated")
	}
	if _, err := legacySpeciesID(0x222); err == nil {
		t.Fatal("wide native species id unexpectedly truncated")
	}
}
