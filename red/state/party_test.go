package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodePartyEmpty(t *testing.T) {
	var m Mem
	p := DecodeParty(&m)
	if p.Count != 0 {
		t.Errorf("Count = %d, want 0", p.Count)
	}
	if len(p.Mons) != 0 {
		t.Errorf("len(Mons) = %d, want 0", len(p.Mons))
	}
}

func TestDecodePartyOneMon(t *testing.T) {
	var m Mem
	m[sym.PartyCount] = 1
	base := sym.PartyMon1
	m[base+sym.MonSpecies] = 4
	m[base+sym.MonLevel] = 12
	m[base+sym.MonHP] = 0x00
	m[base+sym.MonHP+1] = 0x18
	m[base+sym.MonMaxHP] = 0x00
	m[base+sym.MonMaxHP+1] = 0x1F
	m[base+sym.MonStatus] = 0
	copy(m.Slice(base+sym.MonMoves, 4), []byte{33, 45, 0, 0})
	copy(m.Slice(base+sym.MonPP, 4), []byte{35, 40, 0, 0})

	p := DecodeParty(&m)
	if p.Count != 1 || len(p.Mons) != 1 {
		t.Fatalf("Count = %d, len(Mons) = %d, want 1/1", p.Count, len(p.Mons))
	}
	mon := p.Mons[0]
	if mon.Species != 4 {
		t.Errorf("Species = %d, want 4", mon.Species)
	}
	if mon.Level != 12 {
		t.Errorf("Level = %d, want 12", mon.Level)
	}
	// HP is big-endian: 0x0018 = 24. A little-endian read would give 6144.
	if mon.HP != 24 {
		t.Errorf("HP = %d, want 24 (big-endian 0x0018)", mon.HP)
	}
	if mon.MaxHP != 31 {
		t.Errorf("MaxHP = %d, want 31", mon.MaxHP)
	}
	if mon.Status != 0 {
		t.Errorf("Status = %d, want 0", mon.Status)
	}
	if mon.Moves != [4]uint8{33, 45, 0, 0} {
		t.Errorf("Moves = %v, want [33 45 0 0]", mon.Moves)
	}
	if mon.PP != [4]uint8{35, 40, 0, 0} {
		t.Errorf("PP = %v, want [35 40 0 0]", mon.PP)
	}
	if mon.Fainted() {
		t.Errorf("Fainted() = true, want false for HP 24")
	}
}

func TestDecodePartyClampsCount(t *testing.T) {
	var m Mem
	m[sym.PartyCount] = 200
	p := DecodeParty(&m)
	if p.Count != 6 {
		t.Errorf("Count = %d, want 6", p.Count)
	}
	if len(p.Mons) != 6 {
		t.Errorf("len(Mons) = %d, want 6", len(p.Mons))
	}
}

func TestDecodeMoneyBCD(t *testing.T) {
	var m Mem
	m[sym.PlayerMoney] = 0x00
	m[sym.PlayerMoney+1] = 0x12
	m[sym.PlayerMoney+2] = 0x34
	if got := DecodeInventory(&m).Money; got != 1234 {
		t.Errorf("Money = %d, want 1234", got)
	}

	m[sym.PlayerMoney] = 0x99
	m[sym.PlayerMoney+1] = 0x99
	m[sym.PlayerMoney+2] = 0x99
	if got := DecodeInventory(&m).Money; got != 999999 {
		t.Errorf("Money = %d, want 999999", got)
	}
}

func TestDecodeInventory(t *testing.T) {
	var m Mem
	m[sym.NumBagItems] = 2
	items := []byte{4, 3, 20, 1}
	copy(m.Slice(sym.BagItems, len(items)), items)

	inv := DecodeInventory(&m)
	if len(inv.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(inv.Items))
	}
	want := []BagItem{{ID: 4, Quantity: 3}, {ID: 20, Quantity: 1}}
	for i, w := range want {
		if inv.Items[i] != w {
			t.Errorf("Items[%d] = %+v, want %+v", i, inv.Items[i], w)
		}
	}
}

func TestDecodeProgress(t *testing.T) {
	var m Mem
	m[sym.ObtainedBadges] = 0x01
	p := DecodeProgress(&m)
	if p.BadgeCount != 1 {
		t.Errorf("BadgeCount = %d, want 1", p.BadgeCount)
	}
	if !p.Has(BadgeBoulder) {
		t.Errorf("Has(BadgeBoulder) = false, want true")
	}
	if p.Has(BadgeCascade) {
		t.Errorf("Has(BadgeCascade) = true, want false")
	}

	m[sym.ObtainedBadges] = 0xFF
	p = DecodeProgress(&m)
	if p.BadgeCount != 8 {
		t.Errorf("BadgeCount = %d, want 8", p.BadgeCount)
	}
}
