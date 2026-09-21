package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestChooseSafeBagSacrificePrefersCheapestWholeStack(t *testing.T) {
	inv := state.InventoryState{Items: []state.BagItem{
		{ID: 0x04, Quantity: 20}, // POKE BALL: 4000 total
		{ID: 0x14, Quantity: 2},  // POTION: 600 total
		{ID: 0x0B, Quantity: 7},  // ANTIDOTE: 700 total
	}}
	idx, got, ok := chooseSafeBagSacrifice(inv, false)
	if !ok {
		t.Fatal("chooseSafeBagSacrifice returned no candidate")
	}
	if idx != 1 || got.ID != 0x14 || got.Quantity != 2 {
		t.Fatalf("candidate = entry %d %#02x x%d, want entry 1 POTION x2", idx, got.ID, got.Quantity)
	}
}

func TestChooseSafeBagSacrificeProtectsCriticalAndUnknownItems(t *testing.T) {
	protected := []state.BagItem{
		{ID: 0x01, Quantity: 1}, // MASTER BALL
		{ID: 0x05, Quantity: 1}, // TOWN MAP key item
		{ID: 0x2B, Quantity: 1}, // SECRET KEY
		{ID: 0x30, Quantity: 1}, // CARD KEY
		{ID: 0x40, Quantity: 1}, // GOLD TEETH
		{ID: 0x48, Quantity: 1}, // SILPH SCOPE
		{ID: 0x49, Quantity: 1}, // POKE FLUTE
		{ID: 0x4A, Quantity: 1}, // LIFT KEY
		{ID: 0x50, Quantity: 1}, // ETHER (scarce PP recovery)
		{ID: 0xC4, Quantity: 1}, // HM01
		{ID: 0xC8, Quantity: 1}, // HM05
		{ID: 0xC9, Quantity: 1}, // TM01
		{ID: 0xFA, Quantity: 1}, // TM50
		{ID: 0x7F, Quantity: 1}, // unknown / invalid for ordinary bag use
	}
	if idx, item, ok := chooseSafeBagSacrifice(state.InventoryState{Items: protected}, false); ok {
		t.Fatalf("protected-only bag produced candidate entry %d %#02x x%d", idx, item.ID, item.Quantity)
	}
}

func TestChooseSafeBagSacrificeUsesSafeItemAmongProtectedEntries(t *testing.T) {
	inv := state.InventoryState{Items: []state.BagItem{
		{ID: 0x30, Quantity: 1}, // CARD KEY
		{ID: 0xC6, Quantity: 1}, // HM03
		{ID: 0x0B, Quantity: 1}, // ANTIDOTE
		{ID: 0xE2, Quantity: 1}, // TM26
	}}
	idx, got, ok := chooseSafeBagSacrifice(inv, false)
	if !ok || idx != 2 || got.ID != 0x0B {
		t.Fatalf("candidate = (%d, %#02x, %v), want ANTIDOTE at entry 2", idx, got.ID, ok)
	}
}

func TestChooseSafeBagSacrificeReservesEscapeRopeBeforeDig(t *testing.T) {
	inv := state.InventoryState{Items: []state.BagItem{
		{ID: escapeRopeItem, Quantity: 1}, // 550 total; normally cheapest
		{ID: 0x0B, Quantity: 6},           // ANTIDOTE: 600 total
	}}
	if _, got, ok := chooseSafeBagSacrifice(inv, false); !ok || got.ID != escapeRopeItem {
		t.Fatalf("unprotected candidate = %#02x,%v; want Escape Rope", got.ID, ok)
	}
	idx, got, ok := chooseSafeBagSacrifice(inv, true)
	if !ok || idx != 1 || got.ID != 0x0B {
		t.Fatalf("protected candidate = entry %d %#02x,%v; want ANTIDOTE", idx, got.ID, ok)
	}
}

func TestProtectEmergencyEscapeRopeUntilDigExists(t *testing.T) {
	var mem state.Mem
	if !protectEmergencyEscapeRope(&mem) {
		t.Fatal("party without Dig did not reserve Escape Rope")
	}
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = digMoveID
	if protectEmergencyEscapeRope(&mem) {
		t.Fatal("party with Dig still reserved Escape Rope")
	}
}

func TestEnsureBagFreeSlotsRejectsInvalidRequest(t *testing.T) {
	// The bounds check runs before touching the emulator, so nil is safe here.
	for _, want := range []int{-1, gen1BagCapacity + 1} {
		err := EnsureBagFreeSlots(nil, want)
		if err == nil {
			t.Fatalf("EnsureBagFreeSlots(nil, %d) = nil, want error", want)
		}
		if errors.Is(err, ErrNoSafeBagSpace) {
			t.Fatalf("invalid request %d incorrectly classified as ErrNoSafeBagSpace: %v", want, err)
		}
	}
}
