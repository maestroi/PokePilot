package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodePartyReadsCombatFields(t *testing.T) {
	var m Mem
	m[sym.PartyCount] = 1
	base := sym.PartyMon1
	m[base+sym.MonSpecies] = 7
	m[base+sym.MonLevel] = 24
	m[base+sym.MonType1] = 0x15
	m[base+sym.MonType2] = 0x15
	putPartyU16 := func(off uint16, value uint16) {
		m[base+off] = uint8(value >> 8)
		m[base+off+1] = uint8(value)
	}
	putPartyU16(sym.MonHP, 53)
	putPartyU16(sym.MonMaxHP, 71)
	putPartyU16(sym.MonAttack, 48)
	putPartyU16(sym.MonDefense, 62)
	putPartyU16(sym.MonSpeed, 57)
	putPartyU16(sym.MonSpecial, 83)

	party := DecodeParty(&m)
	if len(party.Mons) != 1 {
		t.Fatalf("len(Mons) = %d, want 1", len(party.Mons))
	}
	mon := party.Mons[0]
	if mon.Type1 != 0x15 || mon.Type2 != 0x15 {
		t.Fatalf("types = %#02x/%#02x, want WATER/WATER", mon.Type1, mon.Type2)
	}
	if mon.Attack != 48 || mon.Defense != 62 || mon.Speed != 57 || mon.Special != 83 {
		t.Fatalf("stats = atk %d def %d speed %d special %d, want 48/62/57/83",
			mon.Attack, mon.Defense, mon.Speed, mon.Special)
	}
}
