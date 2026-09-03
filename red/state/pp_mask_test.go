package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeBattleMasksPPUpBits(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 2
	m[sym.BattleMonMoves] = 33
	m[sym.BattleMonPP] = 0xc0 // three PP Ups, zero current PP

	b := DecodeBattle(&m)
	if b == nil {
		t.Fatal("DecodeBattle = nil, want battle")
	}
	if got := b.Moves[0].PP; got != 0 {
		t.Fatalf("Moves[0].PP = %#02x, want 0 current PP after masking PP Up bits", got)
	}
	if got := b.Usable(); len(got) != 0 {
		t.Fatalf("Usable() = %v, want none for raw PP byte 0xc0", got)
	}
}

func TestDecodePartyMasksPPUpBits(t *testing.T) {
	var m Mem
	m[sym.PartyCount] = 1
	m[sym.PartyMon1+sym.MonSpecies] = 1
	m[sym.PartyMon1+sym.MonMoves] = 33
	m[sym.PartyMon1+sym.MonPP] = 0x80 // two PP Ups, zero current PP

	party := DecodeParty(&m)
	if len(party.Mons) != 1 {
		t.Fatalf("party size = %d, want 1", len(party.Mons))
	}
	if got := party.Mons[0].PP[0]; got != 0 {
		t.Fatalf("party PP = %#02x, want 0 current PP after masking PP Up bits", got)
	}
}
