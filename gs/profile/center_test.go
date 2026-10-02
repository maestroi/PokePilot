package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGoldDecodeCenterReportsPartyRecovery(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base] = 0x99 // Bayleef
	mem[base+gen2PartyMovesOffset] = 33
	mem[base+gen2PartyPPOffset] = 10
	putBattleBE16(&mem, base+gen2PartyHPOffset, 80)
	putBattleBE16(&mem, base+gen2PartyMaxHPOffset, 80)

	p := NewGold()
	if !p.DecodeCenter(&mem).Recovered || !p.DecodeCenter(&mem).PartyPresent {
		t.Fatalf("healthy party: %+v", p.DecodeCenter(&mem))
	}

	putBattleBE16(&mem, base+gen2PartyHPOffset, 18)
	if p.DecodeCenter(&mem).Recovered {
		t.Fatal("injured party decoded as recovered")
	}

	putBattleBE16(&mem, base+gen2PartyHPOffset, 80)
	mem[base+gen2PartyPPOffset] = 0
	if p.DecodeCenter(&mem).Recovered {
		t.Fatal("PP-exhausted party decoded as recovered")
	}
}

func TestGoldDecodeCenterIgnoresEggsForRecovery(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 2
	mem[sym.PartySpecies+0] = 0x99 // Bayleef
	mem[sym.PartySpecies+1] = 0xfd // Egg
	lead := sym.PartyMon1
	mem[lead] = 0x99
	mem[lead+gen2PartyMovesOffset] = 33
	mem[lead+gen2PartyPPOffset] = 10
	putBattleBE16(&mem, lead+gen2PartyHPOffset, 80)
	putBattleBE16(&mem, lead+gen2PartyMaxHPOffset, 80)

	// Retail layout: hatch species lives in party_struct with 0 HP; only
	// wPartySpecies carries SPECIES_EGG.
	egg := lead + sym.PartyMonSize
	mem[egg] = 0xaf // Togepi
	putBattleBE16(&mem, egg+gen2PartyHPOffset, 0)
	putBattleBE16(&mem, egg+gen2PartyMaxHPOffset, 19)

	if !NewGold().DecodeCenter(&mem).Recovered {
		t.Fatal("healthy lead plus egg should count as recovered")
	}
}

func TestGoldDecodeCenterStillRecoversWhenPartyStructStoresEggSpecies(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 2
	mem[sym.PartySpecies+0] = 0x99
	lead := sym.PartyMon1
	mem[lead] = 0x99
	mem[lead+gen2PartyMovesOffset] = 33
	mem[lead+gen2PartyPPOffset] = 10
	putBattleBE16(&mem, lead+gen2PartyHPOffset, 80)
	putBattleBE16(&mem, lead+gen2PartyMaxHPOffset, 80)

	egg := lead + sym.PartyMonSize
	mem[egg] = 0xfd

	if !NewGold().DecodeCenter(&mem).Recovered {
		t.Fatal("party_struct SPECIES_EGG should also be skipped")
	}
}

func TestGoldDecodeCenterPromptUsesDrawnYesNo(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.TwoDMenuNumRows] = 2
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	mem[sym.MenuCursorY] = 1
	mem[sym.MenuCursorX] = 1
	putGSScreenText(&mem, "YES NO")

	got := NewGold().DecodeCenter(&mem)
	if !got.PromptOpen || !got.MenuOpen || got.TextOpen {
		t.Fatalf("center prompt = %+v, want prompt/menu without text", got)
	}
}
