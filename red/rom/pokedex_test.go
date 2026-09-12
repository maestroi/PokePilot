package rom

import "testing"

func TestDexNumberInternalSpeciesUsesPokedexOrder(t *testing.T) {
	romData := tmhmTestROM(t)

	got, err := DexNumberInternalSpecies(romData, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0x99 {
		t.Fatalf("dex #1 internal species = %#02x, want %#02x", got, uint8(0x99))
	}
}

func TestDexNumberInternalSpeciesFollowsPatchedOrder(t *testing.T) {
	romData := tmhmTestROM(t)
	romData[pokedexOrderOffset+0x99-1] = 0
	romData[pokedexOrderOffset+0x42-1] = 1

	got, err := DexNumberInternalSpecies(romData, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0x42 {
		t.Fatalf("patched dex #1 internal species = %#02x, want %#02x", got, uint8(0x42))
	}
}

func TestDexNumberInternalSpeciesRejectsMissingAndDuplicateMappings(t *testing.T) {
	romData := tmhmTestROM(t)
	if _, err := DexNumberInternalSpecies(romData, 151); err == nil {
		t.Fatal("unmapped dex #151 unexpectedly resolved")
	}

	romData[pokedexOrderOffset+0x42-1] = 1
	if _, err := DexNumberInternalSpecies(romData, 1); err == nil {
		t.Fatal("duplicate dex #1 mapping unexpectedly resolved")
	}
}
