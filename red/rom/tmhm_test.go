package rom

import "testing"

func tmhmTestROM(t *testing.T) []byte {
	t.Helper()
	romData := make([]byte, pokedexOrderOffset+pokedexOrderLen)
	putMove := func(id uint8) {
		off := movesOffset + (int(id)-1)*moveEntryLen
		romData[off] = id
		romData[off+2] = 50
		romData[off+4] = 255
		romData[off+5] = 20
	}
	// Representative canonical machine moves.
	putMove(5)   // TM01 Mega Punch
	putMove(15)  // HM01 Cut
	putMove(57)  // HM03 Surf
	putMove(148) // HM05 Flash

	romData[technicalMachinesOffset] = 5
	romData[technicalMachinesOffset+50] = 15
	romData[technicalMachinesOffset+52] = 57
	romData[technicalMachinesOffset+54] = 148

	// Internal Bulbasaur is $99 and maps to Pokédex #1.
	romData[pokedexOrderOffset+0x99-1] = 1
	return romData
}

func TestLookupTMHMUsesROMTableAndInventorySemantics(t *testing.T) {
	romData := tmhmTestROM(t)

	tm, err := LookupTMHM(romData, TM01Item)
	if err != nil {
		t.Fatal(err)
	}
	if tm.Number != 1 || tm.Move != 5 || tm.HM || !tm.Consumable() {
		t.Fatalf("TM01 = %+v, want number=1 move=5 consumable TM", tm)
	}

	hm, err := LookupTMHM(romData, HM01Item+2)
	if err != nil {
		t.Fatal(err)
	}
	if hm.Number != 53 || hm.Move != 57 || !hm.HM || hm.Consumable() {
		t.Fatalf("HM03 = %+v, want number=53 move=57 reusable HM", hm)
	}
}

func TestCanLearnTMHMReadsSpeciesCompatibilityBits(t *testing.T) {
	romData := tmhmTestROM(t)
	// HM01 is compatibility flag 51 => zero-based bit 50 => byte 6, bit 2.
	romData[baseStatsOffset+baseStatsTMHMOffset+6] = 1 << 2

	ok, err := CanLearnTMHM(romData, 0x99, HM01Item)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Bulbasaur compatibility bit for HM01 was set but CanLearnTMHM returned false")
	}

	ok, err = CanLearnTMHM(romData, 0x99, HM01Item+2)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("HM03 compatibility bit was clear but CanLearnTMHM returned true")
	}
}

func TestIsHMMoveFollowsPatchedHMTable(t *testing.T) {
	romData := tmhmTestROM(t)

	ok, err := IsHMMove(romData, 57)
	if err != nil || !ok {
		t.Fatalf("IsHMMove(Surf) = %v, %v; want true,nil", ok, err)
	}
	ok, err = IsHMMove(romData, 5)
	if err != nil || ok {
		t.Fatalf("IsHMMove(Mega Punch) = %v, %v; want false,nil", ok, err)
	}

	// Prove this is data-driven rather than a hard-coded canonical HM list.
	romData[technicalMachinesOffset+52] = 5
	ok, err = IsHMMove(romData, 5)
	if err != nil || !ok {
		t.Fatalf("IsHMMove(patched HM03 move) = %v, %v; want true,nil", ok, err)
	}
}

func TestInternalSpeciesDexNumberRejectsMissingNo(t *testing.T) {
	romData := tmhmTestROM(t)
	if _, err := InternalSpeciesDexNumber(romData, 0x1F); err == nil {
		t.Fatal("MissingNo internal slot unexpectedly produced a Pokédex number")
	}
}
