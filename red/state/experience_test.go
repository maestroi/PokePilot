package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestPartyExperienceReadsThreeByteBigEndianValue(t *testing.T) {
	var mem Mem
	mem[sym.PartyCount] = 2
	base := sym.PartyMon1 + sym.PartyMonSize + sym.MonExp
	mem[base], mem[base+1], mem[base+2] = 0x12, 0x34, 0x56

	got, ok := PartyExperience(&mem, 1)
	if !ok || got != 0x123456 {
		t.Fatalf("PartyExperience = %#x,%v, want 0x123456,true", got, ok)
	}
}

func TestPartyExperienceRejectsInvalidSlot(t *testing.T) {
	var mem Mem
	mem[sym.PartyCount] = 1
	if _, ok := PartyExperience(&mem, -1); ok {
		t.Fatal("negative party slot accepted")
	}
	if _, ok := PartyExperience(&mem, 1); ok {
		t.Fatal("slot beyond party accepted")
	}
}
