package rom

import "testing"

func TestFishingEncountersReadsAllThreeRods(t *testing.T) {
	romData := make([]byte, 0x10000)
	oldOff, err := bankedOffset(itemUseOldRodBank, itemUseOldRodAddr)
	if err != nil {
		t.Fatal(err)
	}
	romData[oldOff+oldRodOpcodeOffset] = 0x01
	romData[oldOff+oldRodOpcodeOffset+1] = 0x85 // Magikarp
	romData[oldOff+oldRodOpcodeOffset+2] = 5

	goodOff, err := bankedOffset(goodRodMonsBank, goodRodMonsAddr)
	if err != nil {
		t.Fatal(err)
	}
	romData[goodOff] = 10
	romData[goodOff+1] = 0x9D // Goldeen
	romData[goodOff+2] = 10
	romData[goodOff+3] = 0x47 // Poliwag

	superOff, err := bankedOffset(superRodDataBank, superRodDataAddr)
	if err != nil {
		t.Fatal(err)
	}
	groupAddr := uint16(0x6980)
	romData[superOff] = mapPalletTown
	romData[superOff+1] = byte(groupAddr)
	romData[superOff+2] = byte(groupAddr >> 8)
	romData[superOff+3] = 0xFF
	group, err := bankedOffset(superRodDataBank, groupAddr)
	if err != nil {
		t.Fatal(err)
	}
	romData[group] = 1
	romData[group+1] = 15
	romData[group+2] = 0x18 // Tentacool

	got, err := FishingEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("fishing = %+v, want old+2 good+1 super", got)
	}
	if got[0] != (FishingEncounter{Rod: RodOld, Species: 0x85, Level: 5}) {
		t.Fatalf("old rod = %+v, want Magikarp lv5", got[0])
	}
	if got[3] != (FishingEncounter{MapID: mapPalletTown, Rod: RodSuper, Species: 0x18, Level: 15}) {
		t.Fatalf("super rod = %+v, want Pallet Tentacool lv15", got[3])
	}
}

func TestFishingEncountersFollowsPatchedGoodRod(t *testing.T) {
	romData := make([]byte, 0x10000)
	oldOff, _ := bankedOffset(itemUseOldRodBank, itemUseOldRodAddr)
	romData[oldOff+oldRodOpcodeOffset] = 0x01
	romData[oldOff+oldRodOpcodeOffset+1] = 0x85
	romData[oldOff+oldRodOpcodeOffset+2] = 5
	goodOff, _ := bankedOffset(goodRodMonsBank, goodRodMonsAddr)
	romData[goodOff] = 12
	romData[goodOff+1] = 0x2F // Psyduck
	romData[goodOff+2] = 12
	romData[goodOff+3] = 0x2F
	superOff, _ := bankedOffset(superRodDataBank, superRodDataAddr)
	romData[superOff] = 0xFF

	got, err := FishingEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1].Species != 0x2F || got[1].Rod != RodGood {
		t.Fatalf("patched good rod = %+v, want Psyduck", got)
	}
}

func TestFishingEncountersFromROM(t *testing.T) {
	romData := loadROM(t)
	got, err := FishingEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFish(got, RodOld, 0x85) {
		t.Fatalf("old rod missing Magikarp: %+v", got)
	}
	if !hasFish(got, RodGood, 0x9D) || !hasFish(got, RodGood, 0x47) {
		t.Fatalf("good rod missing Goldeen/Poliwag: %+v", got)
	}
	if !hasFish(got, RodSuper, 0x85) {
		t.Fatalf("super rod missing Magikarp: %+v", got)
	}
}

func hasFish(enc []FishingEncounter, rod string, species uint8) bool {
	for _, e := range enc {
		if e.Rod == rod && e.Species == species {
			return true
		}
	}
	return false
}
