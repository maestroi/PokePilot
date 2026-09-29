package rom

import (
	"testing"

	gsdata "github.com/maestroi/pokepilot/gs/data"
)

func syntheticBaseDataROM(t *testing.T) ([]byte, int) {
	t.Helper()
	base := 0x120
	romData := make([]byte, base+gen2SpeciesCount*gen2BaseDataSize+0x20)
	copy(romData[base:], []byte{0x01, 45, 49, 49, 45, 65, 65})
	copy(romData[base+gen2BaseDataSize:], []byte{0x02, 60, 62, 63, 60, 80, 80})
	copy(romData[base+2*gen2BaseDataSize:], []byte{0x03, 80, 82, 83, 80, 100, 100})
	return romData, base
}

func setMachineCompatibility(t *testing.T, romData []byte, base int, species, item uint8) {
	t.Helper()
	machine, ok := MachineNumberForItem(item)
	if !ok {
		t.Fatalf("item %#02x is not a machine", item)
	}
	off := base + (int(species)-1)*gen2BaseDataSize + gen2TMHMOffset
	bit := machine - 1
	romData[off+bit/8] |= 1 << uint(bit%8)
}

func TestParseTMHMCompatibilityReadsNativeBits(t *testing.T) {
	romData, base := syntheticBaseDataROM(t)
	const (
		tmHeadbutt = uint8(0xc0)
		hmCut      = uint8(0xf3)
	)
	setMachineCompatibility(t, romData, base, 1, tmHeadbutt)
	setMachineCompatibility(t, romData, base, 1, hmCut)

	table, err := ParseTMHMCompatibility(romData)
	if err != nil {
		t.Fatalf("ParseTMHMCompatibility: %v", err)
	}
	for _, item := range []uint8{tmHeadbutt, hmCut} {
		ok, err := table.CanLearn(1, item)
		if err != nil {
			t.Fatalf("CanLearn(Bulbasaur, %#02x): %v", item, err)
		}
		if !ok {
			t.Fatalf("Bulbasaur compatibility for %#02x was set but returned false", item)
		}
	}
	ok, err := table.CanLearn(2, hmCut)
	if err != nil {
		t.Fatalf("CanLearn(Ivysaur, HM01): %v", err)
	}
	if ok {
		t.Fatal("unset Ivysaur HM01 compatibility returned true")
	}
}

func TestMachineNumberForItemCoversFullGen2Pocket(t *testing.T) {
	if got, ok := MachineNumberForItem(gsdata.MachineItems[0]); !ok || got != 1 {
		t.Fatalf("TM01 machine number = %d, %v; want 1,true", got, ok)
	}
	if got, ok := MachineNumberForItem(0xf3); !ok || got != 51 {
		t.Fatalf("HM01 machine number = %d, %v; want 51,true", got, ok)
	}
	if got, ok := MachineNumberForItem(0xf9); !ok || got != 57 {
		t.Fatalf("HM07 machine number = %d, %v; want 57,true", got, ok)
	}
	if _, ok := MachineNumberForItem(0x01); ok {
		t.Fatal("ordinary item reported as TM/HM")
	}
}

func TestParseTMHMCompatibilityFailsClosed(t *testing.T) {
	if _, err := ParseTMHMCompatibility(make([]byte, 4096)); err == nil {
		t.Fatal("missing BaseData signature unexpectedly parsed")
	}

	romData, base := syntheticBaseDataROM(t)
	second := base + gen2SpeciesCount*gen2BaseDataSize + 1
	need := second + 3*gen2BaseDataSize
	if need > len(romData) {
		romData = append(romData, make([]byte, need-len(romData))...)
	}
	copy(romData[second:], romData[base:base+3*gen2BaseDataSize])
	if _, err := ParseTMHMCompatibility(romData); err == nil {
		t.Fatal("ambiguous BaseData signature unexpectedly parsed")
	}
}
