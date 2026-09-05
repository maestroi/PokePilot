package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	testTechnicalMachinesOffset = 0x13773
	testPokedexOrderOffset       = 0x41024
	testBaseStatsOffset          = 0x383DE
	testBaseStatsEntryLen        = 28
	testBaseStatsTMHMOffset      = 20
)

func fakeTMHMROM(t *testing.T, machineItem, machineMove uint8, moves ...rom.Move) []byte {
	t.Helper()
	base := fakeROM(t, moves...)
	size := testPokedexOrderOffset + 190
	romData := make([]byte, size)
	copy(romData, base)

	number, _, err := rom.MachineNumber(machineItem)
	if err != nil {
		t.Fatal(err)
	}
	romData[testTechnicalMachinesOffset+number-1] = machineMove
	return romData
}

func allowTMHM(t *testing.T, romData []byte, internalSpecies, dex, item uint8) {
	t.Helper()
	number, _, err := rom.MachineNumber(item)
	if err != nil {
		t.Fatal(err)
	}
	romData[testPokedexOrderOffset+int(internalSpecies)-1] = dex
	flag := number - 1
	off := testBaseStatsOffset + (int(dex)-1)*testBaseStatsEntryLen + testBaseStatsTMHMOffset + flag/8
	romData[off] |= 1 << uint(flag%8)
}

func TestDecideTMHMPicksCompatibleRecipientAndImprovement(t *testing.T) {
	tackle := rom.Move{ID: 33, Power: 35, Type: learnTypeNormal, Accuracy: 255, PP: 35}
	growl := rom.Move{ID: 45, Effect: rom.AttackDown1Effect, Accuracy: 255, PP: 40}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	vineWhip := rom.Move{ID: 22, Power: 35, Type: learnTypeGrass, Accuracy: 255, PP: 10}
	thunderbolt := rom.Move{ID: 85, Power: 95, Type: 0x17, Accuracy: 255, PP: 15}
	romData := fakeTMHMROM(t, rom.TM01Item, thunderbolt.ID, tackle, growl, leechSeed, vineWhip, thunderbolt)

	// Slot 0 is deliberately incompatible. Slot 1 maps to dex #1 and owns
	// TM01's compatibility bit.
	romData[testPokedexOrderOffset+0xB0-1] = 4
	allowTMHM(t, romData, 0x99, 1, rom.TM01Item)
	party := state.PartyState{Count: 2, Mons: []state.Mon{
		{Species: 0xB0, Type1: learnTypeNormal, Type2: learnTypeNormal, Moves: [4]uint8{tackle.ID}},
		{Species: 0x99, Type1: learnTypeGrass, Type2: learnTypePoison, Moves: [4]uint8{tackle.ID, growl.ID, leechSeed.ID, vineWhip.ID}},
	}}

	d, err := DecideTMHM(romData, party, rom.TM01Item, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.PartySlot != 1 {
		t.Fatalf("party slot = %d, want compatible slot 1: %+v", d.PartySlot, d)
	}
	if d.AfterScore-d.BeforeScore < minMoveLearningImprovement {
		t.Fatalf("optional TM was proposed without material improvement: %+v", d)
	}
	if d.ReplaceSlot < 0 {
		t.Fatalf("full move set did not choose a replacement: %+v", d)
	}
}

func TestDecideTMHMNeverReplacesROMDefinedHM(t *testing.T) {
	cut := rom.Move{ID: 15, Power: 50, Type: learnTypeNormal, Accuracy: 242, PP: 30}
	growl := rom.Move{ID: 45, Effect: rom.AttackDown1Effect, Accuracy: 255, PP: 40}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	vineWhip := rom.Move{ID: 22, Power: 35, Type: learnTypeGrass, Accuracy: 255, PP: 10}
	thunderbolt := rom.Move{ID: 85, Power: 95, Type: 0x17, Accuracy: 255, PP: 15}
	romData := fakeTMHMROM(t, rom.TM01Item, thunderbolt.ID, cut, growl, leechSeed, vineWhip, thunderbolt)
	// Mark Cut as an HM in the loaded ROM, rather than relying on its move id.
	romData[testTechnicalMachinesOffset+rom.NumTMs] = cut.ID
	allowTMHM(t, romData, 0x99, 1, rom.TM01Item)

	d, err := DecideTMHM(romData, state.PartyState{Count: 1, Mons: []state.Mon{{
		Species: 0x99, Type1: learnTypeGrass, Type2: learnTypePoison,
		Moves: [4]uint8{cut.ID, growl.ID, leechSeed.ID, vineWhip.ID},
	}}}, rom.TM01Item, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.ReplaceSlot == 0 {
		t.Fatalf("ROM-defined HM Cut was selected for replacement: %+v", d)
	}
}

func TestDecideTMHMDoesNotSpendTMOnBadUpgrade(t *testing.T) {
	tackle := rom.Move{ID: 33, Power: 35, Type: learnTypeNormal, Accuracy: 255, PP: 35}
	bodySlam := rom.Move{ID: 34, Power: 85, Type: learnTypeNormal, Accuracy: 255, PP: 15}
	vineWhip := rom.Move{ID: 22, Power: 35, Type: learnTypeGrass, Accuracy: 255, PP: 10}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	splash := rom.Move{ID: 150, Effect: 0x55, Accuracy: 255, PP: 40}
	romData := fakeTMHMROM(t, rom.TM01Item, splash.ID, tackle, bodySlam, vineWhip, leechSeed, splash)
	allowTMHM(t, romData, 0x99, 1, rom.TM01Item)
	party := state.PartyState{Count: 1, Mons: []state.Mon{{
		Species: 0x99, Type1: learnTypeGrass, Type2: learnTypePoison,
		Moves: [4]uint8{tackle.ID, bodySlam.ID, vineWhip.ID, leechSeed.ID},
	}}}

	if d, err := DecideTMHM(romData, party, rom.TM01Item, false); err == nil {
		t.Fatalf("bad consumable TM unexpectedly proposed: %+v", d)
	}
}

func TestDecideTMHMRequiredFieldMoveCanTradeBattleScore(t *testing.T) {
	tackle := rom.Move{ID: 33, Power: 35, Type: learnTypeNormal, Accuracy: 255, PP: 35}
	bodySlam := rom.Move{ID: 34, Power: 85, Type: learnTypeNormal, Accuracy: 255, PP: 15}
	vineWhip := rom.Move{ID: 22, Power: 35, Type: learnTypeGrass, Accuracy: 255, PP: 10}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	flash := rom.Move{ID: 148, Effect: 0x01, Accuracy: 178, PP: 20}
	romData := fakeTMHMROM(t, rom.HM05Item, flash.ID, tackle, bodySlam, vineWhip, leechSeed, flash)
	allowTMHM(t, romData, 0x99, 1, rom.HM05Item)
	party := state.PartyState{Count: 1, Mons: []state.Mon{{
		Species: 0x99, Type1: learnTypeGrass, Type2: learnTypePoison,
		Moves: [4]uint8{tackle.ID, bodySlam.ID, vineWhip.ID, leechSeed.ID},
	}}}

	if d, err := DecideTMHM(romData, party, rom.HM05Item, false); err == nil {
		t.Fatalf("optional low-value permanent HM unexpectedly proposed: %+v", d)
	}
	d, err := DecideTMHM(romData, party, rom.HM05Item, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.PartySlot != 0 || d.ReplaceSlot < 0 {
		t.Fatalf("required HM did not choose a concrete recipient/replacement: %+v", d)
	}
}

func TestDecideTMHMIsIdempotentWhenMoveAlreadyKnown(t *testing.T) {
	thunderbolt := rom.Move{ID: 85, Power: 95, Type: 0x17, Accuracy: 255, PP: 15}
	romData := fakeTMHMROM(t, rom.TM01Item, thunderbolt.ID, thunderbolt)
	allowTMHM(t, romData, 0x99, 1, rom.TM01Item)

	d, err := DecideTMHM(romData, state.PartyState{Count: 1, Mons: []state.Mon{{
		Species: 0x99, Type1: 0x17, Type2: 0x17, Moves: [4]uint8{thunderbolt.ID},
	}}}, rom.TM01Item, false)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Existing || d.PartySlot != 0 {
		t.Fatalf("already-known machine decision = %+v", d)
	}
}
