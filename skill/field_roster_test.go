package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func setTestMachineMove(t *testing.T, romData []byte, item, move uint8) {
	t.Helper()
	number, _, err := rom.MachineNumber(item)
	if err != nil {
		t.Fatal(err)
	}
	romData[testTechnicalMachinesOffset+number-1] = move
}

func fakeCoreFieldROM(t *testing.T) []byte {
	t.Helper()
	cut, _ := FieldMoveSpecFor(FieldCut)
	surf, _ := FieldMoveSpecFor(FieldSurf)
	strength, _ := FieldMoveSpecFor(FieldStrength)
	romData := fakeTMHMROM(t, cut.HMItem, cut.MoveID,
		rom.Move{ID: cut.MoveID, Power: 50, Type: learnTypeNormal, Accuracy: 242, PP: 30},
		rom.Move{ID: surf.MoveID, Power: 95, Type: 0x15, Accuracy: 255, PP: 15},
		rom.Move{ID: strength.MoveID, Power: 80, Type: learnTypeNormal, Accuracy: 255, PP: 15},
		rom.Move{ID: 33, Power: 35, Type: learnTypeNormal, Accuracy: 255, PP: 35},
	)
	setTestMachineMove(t, romData, surf.HMItem, surf.MoveID)
	setTestMachineMove(t, romData, strength.HMItem, strength.MoveID)
	// The roster planner asks ROM compatibility about every hypothetical party
	// member, including deliberately incompatible filler mons. Give those fake
	// internal species valid Pokédex mappings with no HM compatibility bits.
	for i, species := range []uint8{0x10, 0x11, 0x12, 0x13, 0x14, 0x15} {
		romData[testPokedexOrderOffset+int(species)-1] = uint8(10 + i)
	}
	return romData
}

func TestOwnedSurfWithoutCompatiblePartyNeedsRosterRepair(t *testing.T) {
	surf, _ := FieldMoveSpecFor(FieldSurf)
	romData := fakeTMHMROM(t, surf.HMItem, surf.MoveID,
		rom.Move{ID: surf.MoveID, Power: 95, Type: 0x15, Accuracy: 255, PP: 15})
	const species = 0x54
	romData[testPokedexOrderOffset+species-1] = 1 // valid species mapping, HM bit deliberately clear

	var mem state.Mem
	mem[sym.ObtainedBadges] = 1 << uint8(surf.Badge)
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = surf.HMItem
	mem[sym.BagItems+1] = 1
	mem[sym.PartyCount] = 1
	mem[sym.PartySpecies] = species
	mem[sym.PartyMon1+sym.MonSpecies] = species

	cap := FieldCapabilityFor(&mem, FieldSurf)
	if !cap.BadgeOwned || !cap.HMOwned || cap.Learned || cap.Usable {
		t.Fatalf("Surf capability = %+v, want owned/unlocked but not learned/usable", cap)
	}
	if CanPrepareFieldMove(romData, &mem, FieldSurf) {
		t.Fatal("CanPrepareFieldMove returned true with no Surf-compatible party member")
	}
	owned := OwnedCoreProgressionFieldMoves(&mem)
	if len(owned) != 1 || owned[0] != FieldSurf {
		t.Fatalf("owned core field moves = %v, want [SURF]", owned)
	}
}

func TestChooseDepositSlotPreservesCutSurfStrengthInvariant(t *testing.T) {
	romData := fakeCoreFieldROM(t)
	cut, _ := FieldMoveSpecFor(FieldCut)
	surf, _ := FieldMoveSpecFor(FieldSurf)
	strength, _ := FieldMoveSpecFor(FieldStrength)

	const surfSpecies = 0x20
	allowTMHM(t, romData, surfSpecies, 1, surf.HMItem)

	party := state.PartyState{Count: 6, Mons: []state.Mon{
		{Species: 0x10, Level: 4, Moves: [4]uint8{cut.MoveID}},      // only current Cut user: do not sacrifice
		{Species: 0x11, Level: 6, Moves: [4]uint8{strength.MoveID}}, // only current Strength user
		{Species: 0x12, Level: 2, Moves: [4]uint8{33}},              // weakest safe filler: should be stored
		{Species: 0x13, Level: 8, Moves: [4]uint8{33}},
		{Species: 0x14, Level: 9, Moves: [4]uint8{33}},
		{Species: 0x15, Level: 10, Moves: [4]uint8{33}},
	}}
	incoming := state.Mon{Species: surfSpecies}
	slot, ok, err := chooseDepositSlotForIncoming(romData, party, incoming, []FieldMove{FieldCut, FieldSurf, FieldStrength})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || slot != 2 {
		t.Fatalf("deposit choice = slot %d ok=%v, want safe filler slot 2", slot, ok)
	}

	mons := append([]state.Mon(nil), party.Mons[:slot]...)
	mons = append(mons, party.Mons[slot+1:]...)
	mons = append(mons, incoming)
	covered, err := partyCanSatisfyFieldMoves(romData, mons, []FieldMove{FieldCut, FieldSurf, FieldStrength})
	if err != nil {
		t.Fatal(err)
	}
	if !covered {
		t.Fatal("post-swap party cannot satisfy Cut+Surf+Strength")
	}
}

func TestChooseDepositSlotRefusesSwapThatStrandsCut(t *testing.T) {
	romData := fakeCoreFieldROM(t)
	cut, _ := FieldMoveSpecFor(FieldCut)
	surf, _ := FieldMoveSpecFor(FieldSurf)
	const surfSpecies = 0x20
	allowTMHM(t, romData, surfSpecies, 1, surf.HMItem)

	// Every member except slot 0 is made indispensable by giving it a required
	// move and no compatibility elsewhere. There is no legal six-for-one swap
	// that can retain the whole set once the only Cut user is removed.
	party := state.PartyState{Count: 6, Mons: []state.Mon{
		{Species: 0x10, Moves: [4]uint8{cut.MoveID}},
		{Species: 0x11, Moves: [4]uint8{33}},
		{Species: 0x12, Moves: [4]uint8{33}},
		{Species: 0x13, Moves: [4]uint8{33}},
		{Species: 0x14, Moves: [4]uint8{33}},
		{Species: 0x15, Moves: [4]uint8{33}},
	}}
	// Requiring Cut and Surf still permits depositing a filler, but never the
	// only Cut user. Make the Cut user the weakest to prove preservation wins
	// over the normal weakest-member preference.
	party.Mons[0].Level = 1
	for i := 1; i < len(party.Mons); i++ {
		party.Mons[i].Level = uint8(10 + i)
	}
	slot, ok, err := chooseDepositSlotForIncoming(romData, party, state.Mon{Species: surfSpecies}, []FieldMove{FieldCut, FieldSurf})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a filler slot to remain legal")
	}
	if slot == 0 {
		t.Fatal("planner chose the only Cut user for deposit")
	}
}

func TestChooseCompatibleBoxMonPrefersBroaderFieldCoverage(t *testing.T) {
	romData := fakeCoreFieldROM(t)
	surf, _ := FieldMoveSpecFor(FieldSurf)
	strength, _ := FieldMoveSpecFor(FieldStrength)
	const (
		surfOnly = 0x20
		multi    = 0x21
	)
	allowTMHM(t, romData, surfOnly, 1, surf.HMItem)
	allowTMHM(t, romData, multi, 2, surf.HMItem)
	allowTMHM(t, romData, multi, 2, strength.HMItem)

	party := state.PartyState{Count: 2, Mons: []state.Mon{{Species: 0x10}, {Species: 0x11}}}
	box := state.BoxState{Count: 2, Mons: []state.BoxMon{{Species: surfOnly}, {Species: multi}}}
	idx, deposit, ok, err := chooseCompatibleBoxMon(romData, party, box, FieldSurf, []FieldMove{FieldSurf, FieldStrength})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || idx != 1 || deposit != -1 {
		t.Fatalf("box choice = index %d deposit %d ok=%v, want broader candidate index 1 with no deposit", idx, deposit, ok)
	}
}
