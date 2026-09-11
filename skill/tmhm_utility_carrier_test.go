package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

func TestDecideTMHMRequiredFlashPrefersBenchCarrier(t *testing.T) {
	flash := rom.Move{ID: 148, Effect: 0x01, Accuracy: 178, PP: 20}
	romData := fakeTMHMROM(t, rom.HM05Item, flash.ID, flash)
	allowTMHM(t, romData, 0x99, 1, rom.HM05Item)
	allowTMHM(t, romData, 0xB0, 4, rom.HM05Item)

	party := state.PartyState{Count: 2, Mons: []state.Mon{
		{Species: 0x99, Moves: [4]uint8{}},
		{Species: 0xB0, Moves: [4]uint8{}},
	}}
	d, err := DecideTMHM(romData, party, rom.HM05Item, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.PartySlot != 1 {
		t.Fatalf("required Flash recipient = party slot %d, want bench slot 1: %+v", d.PartySlot, d)
	}
}

func TestDecideTMHMRequiredCutPrefersBenchCarrier(t *testing.T) {
	cut := rom.Move{ID: cutMove, Power: 50, Type: learnTypeNormal, Accuracy: 242, PP: 30}
	romData := fakeTMHMROM(t, hm01Item, cut.ID, cut)
	allowTMHM(t, romData, 0x99, 1, hm01Item)
	allowTMHM(t, romData, 0xB0, 4, hm01Item)

	party := state.PartyState{Count: 2, Mons: []state.Mon{
		{Species: 0x99, Moves: [4]uint8{}},
		{Species: 0xB0, Moves: [4]uint8{}},
	}}
	d, err := DecideTMHM(romData, party, hm01Item, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.PartySlot != 1 {
		t.Fatalf("required Cut recipient = party slot %d, want bench slot 1: %+v", d.PartySlot, d)
	}
}

func TestDecideTMHMRequiredUtilityHMStillFallsBackToLead(t *testing.T) {
	flash := rom.Move{ID: 148, Effect: 0x01, Accuracy: 178, PP: 20}
	romData := fakeTMHMROM(t, rom.HM05Item, flash.ID, flash)
	allowTMHM(t, romData, 0x99, 1, rom.HM05Item)

	party := state.PartyState{Count: 1, Mons: []state.Mon{{Species: 0x99, Moves: [4]uint8{}}}}
	d, err := DecideTMHM(romData, party, rom.HM05Item, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.PartySlot != 0 {
		t.Fatalf("single compatible lead should remain progression fallback, got slot %d: %+v", d.PartySlot, d)
	}
}
