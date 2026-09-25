package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

type captureTestMemory [0x10000]byte

func (m *captureTestMemory) Peek8(addr uint16) byte { return (*m)[addr] }

func (m *captureTestMemory) PeekInto(addr uint16, dst []byte) {
	copy(dst, (*m)[int(addr):])
}

func TestDecodeInventoryProjectsNativeItems(t *testing.T) {
	m := new(captureTestMemory)
	m[sym.NumBagItems] = 2
	m[sym.BagItems] = 0x02
	m[sym.BagItems+1] = 3
	m[sym.BagItems+2] = 0x03
	m[sym.BagItems+3] = 7
	m[sym.PlayerMoney] = 0x12
	m[sym.PlayerMoney+1] = 0x34
	m[sym.PlayerMoney+2] = 0x56

	got := New().DecodeInventory(m)
	if got.Money != 123456 {
		t.Fatalf("money = %d, want 123456", got.Money)
	}
	if len(got.Items) != 2 || got.Items[0].NativeItemID != 0x02 || got.Items[0].Quantity != 3 ||
		got.Items[1].NativeItemID != 0x03 || got.Items[1].Quantity != 7 {
		t.Fatalf("inventory = %+v", got.Items)
	}
}

func TestDecodeCaptureProjectsPartyBoxAndOwnedDex(t *testing.T) {
	m := new(captureTestMemory)
	m[sym.PartyCount] = 1
	m[sym.PartyMon1+sym.MonSpecies] = 0x54
	m[sym.BoxCount] = 1
	m[sym.BoxMon1+sym.BoxMonSpecies] = 0x99

	// Pokédex #25 owned => bit 24 in the owned bitset.
	m[sym.PokedexOwned+3] = 1

	got := New().DecodeCapture(m)
	if len(got.PartySpecies) != 1 || got.PartySpecies[0] != 0x54 {
		t.Fatalf("party species = %+v", got.PartySpecies)
	}
	if len(got.ActiveBoxSpecies) != 1 || got.ActiveBoxSpecies[0] != 0x99 {
		t.Fatalf("box species = %+v", got.ActiveBoxSpecies)
	}
	if len(got.OwnedDex) != 1 || got.OwnedDex[0] != 25 {
		t.Fatalf("owned dex = %+v, want [25]", got.OwnedDex)
	}
}

func TestOrdinaryCaptureBallOrderExcludesMasterBall(t *testing.T) {
	got := New().OrdinaryCaptureBallOrder()
	want := []uint16{0x02, 0x03, 0x04}
	if len(got) != len(want) {
		t.Fatalf("ball order = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ball order = %+v, want %+v", got, want)
		}
	}
}
