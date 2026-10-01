package rom

import "testing"

const testBaseDataOffset = 0x240

func tmhmTestROM() []byte {
	rom := make([]byte, testBaseDataOffset+gen2PokemonCount*gen2BaseDataEntrySize+0x100)
	for species := 1; species <= gen2PokemonCount; species++ {
		rom[testBaseDataOffset+(species-1)*gen2BaseDataEntrySize] = byte(species)
	}
	return rom
}

func allowMachine(rom []byte, species uint8, machineNumber int) {
	flag := machineNumber - 1
	offset := testBaseDataOffset +
		(int(species)-1)*gen2BaseDataEntrySize +
		gen2BaseDataTMHMOffset +
		flag/8
	rom[offset] |= 1 << uint(flag%8)
}

func TestLocateBaseDataUsesCompleteDexStride(t *testing.T) {
	rom := tmhmTestROM()
	base, err := LocateBaseData(rom)
	if err != nil {
		t.Fatalf("LocateBaseData: %v", err)
	}
	if base != testBaseDataOffset {
		t.Fatalf("BaseData offset=%#x, want %#x", base, testBaseDataOffset)
	}
}

func TestCanLearnTMHMReadsGen2CompatibilityBits(t *testing.T) {
	rom := tmhmTestROM()
	const chikorita = uint8(0x98)

	allowMachine(rom, chikorita, 2)  // TM02 Headbutt
	allowMachine(rom, chikorita, 51) // HM01 Cut
	allowMachine(rom, chikorita, 57) // HM07 Waterfall

	for _, tc := range []struct {
		machine int
		want    bool
	}{
		{2, true},
		{3, false},
		{51, true},
		{52, false},
		{57, true},
	} {
		got, err := CanLearnTMHM(rom, chikorita, tc.machine)
		if err != nil {
			t.Fatalf("CanLearnTMHM(machine=%d): %v", tc.machine, err)
		}
		if got != tc.want {
			t.Fatalf("CanLearnTMHM(machine=%d)=%v, want %v", tc.machine, got, tc.want)
		}
	}
}

func TestCanLearnTMHMRejectsUnsupportedInputs(t *testing.T) {
	rom := tmhmTestROM()
	for _, tc := range []struct {
		species uint8
		machine int
	}{
		{0, 1},
		{1, 0},
		{1, 58},
		{0xfc, 1},
	} {
		if _, err := CanLearnTMHM(rom, tc.species, tc.machine); err == nil {
			t.Fatalf("CanLearnTMHM(species=%#02x,machine=%d) accepted invalid input", tc.species, tc.machine)
		}
	}

	if _, err := LocateBaseData(make([]byte, gen2PokemonCount*gen2BaseDataEntrySize)); err == nil {
		t.Fatal("LocateBaseData accepted zero-filled ROM")
	}
}
