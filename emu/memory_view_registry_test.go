package emu

import "testing"

// constView answers every canonical read with one byte, so a test can see
// whether Peek8 went through it.
type constView byte

func (v constView) ReadInto(_ func(uint16, []byte), _ uint16, dst []byte) {
	for i := range dst {
		dst[i] = byte(v)
	}
}

// syntheticROM is a minimal 32 KiB ROM-only cartridge whose header byte 0x134
// tags it for the resolver below.
func syntheticROM(tag byte) []byte {
	rom := make([]byte, 0x8000)
	rom[0x100], rom[0x101], rom[0x102], rom[0x103] = 0x00, 0xC3, 0x00, 0x01 // nop; jp $0100
	rom[0x134] = tag
	var sum byte
	for _, b := range rom[0x134:0x14D] {
		sum = sum - b - 1
	}
	rom[0x14D] = sum
	return rom
}

const viewTag = 0x5A

func init() {
	RegisterMemoryViewResolver(func(rom []byte) MemoryView {
		if len(rom) > 0x134 && rom[0x134] == viewTag {
			return constView(0xAB)
		}
		return nil
	})
}

func TestLoadingACartridgeBindsItsRegisteredView(t *testing.T) {
	m, err := OpenCGBBytes(syntheticROM(viewTag))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if got := m.Peek8(0xC000); got != 0xAB {
		t.Fatalf("tagged cartridge Peek8 = %#02x, want the registered view's 0xab", got)
	}
	if got := m.Peek8Native(0xC000); got == 0xAB {
		t.Fatal("native read went through the view")
	}

	// Reloading an untagged cartridge must drop the previous game's view.
	if err := m.LoadROMBytes(syntheticROM(0), "plain"); err != nil {
		t.Fatal(err)
	}
	if got := m.Peek8(0xC000); got == 0xAB {
		t.Fatal("untagged cartridge kept the previous cartridge's view")
	}
}
