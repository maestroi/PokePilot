package rom

import (
	"os"
	"testing"
)

// Map ids from constants/map_constants.asm.
const (
	mapPalletTown  uint8 = 0x00
	mapRoute1      uint8 = 0x0C
	mapRoute21     uint8 = 0x20
	mapRedsHouse1F uint8 = 0x25
	mapBluesHouse  uint8 = 0x27
	mapOaksLab     uint8 = 0x28
)

func loadROM(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	return rom
}

func TestParsePalletTown(t *testing.T) {
	rom := loadROM(t)
	h, err := ParseMap(rom, mapPalletTown)
	if err != nil {
		t.Fatalf("ParseMap(PalletTown) = %v, want nil", err)
	}

	wantWarps := []Warp{
		{X: 5, Y: 5, DestWarpID: 0, DestMap: mapRedsHouse1F},
		{X: 13, Y: 5, DestWarpID: 0, DestMap: mapBluesHouse},
		{X: 12, Y: 11, DestWarpID: 1, DestMap: mapOaksLab},
	}
	if len(h.Warps) != len(wantWarps) {
		t.Fatalf("len(Warps) = %d, want %d: %#v", len(h.Warps), len(wantWarps), h.Warps)
	}
	for i, w := range h.Warps {
		if w != wantWarps[i] {
			t.Errorf("Warps[%d] = %#v, want %#v", i, w, wantWarps[i])
		}
	}

	if len(h.Signs) != 4 {
		t.Errorf("len(Signs) = %d, want 4: %#v", len(h.Signs), h.Signs)
	}
	if len(h.Objects) != 3 {
		t.Errorf("len(Objects) = %d, want 3: %#v", len(h.Objects), h.Objects)
	}
	if h.BorderBlock != 0x0B {
		t.Errorf("BorderBlock = %#x, want 0x0B", h.BorderBlock)
	}

	wantConns := []Connection{
		{Dir: 0, MapID: mapRoute1},  // north
		{Dir: 1, MapID: mapRoute21}, // south
	}
	if len(h.Connections) != len(wantConns) {
		t.Fatalf("len(Connections) = %d, want %d: %#v", len(h.Connections), len(wantConns), h.Connections)
	}
	for i, c := range h.Connections {
		if c != wantConns[i] {
			t.Errorf("Connections[%d] = %#v, want %#v", i, c, wantConns[i])
		}
	}
}

func TestParsePalletTownBlocks(t *testing.T) {
	rom := loadROM(t)
	h, err := ParseMap(rom, mapPalletTown)
	if err != nil {
		t.Fatalf("ParseMap(PalletTown) = %v, want nil", err)
	}
	blocks, err := Blocks(rom, h)
	if err != nil {
		t.Fatalf("Blocks(PalletTown) = %v, want nil", err)
	}
	want := int(h.WidthBlocks) * int(h.HeightBlocks)
	if len(blocks) != want {
		t.Errorf("len(Blocks) = %d, want %d", len(blocks), want)
	}
}

func TestParseAllMapsDoesNotPanic(t *testing.T) {
	rom := loadROM(t)
	succeeded := 0
	for id := 0; id <= 247; id++ {
		_, err := ParseMap(rom, uint8(id))
		if err == nil {
			succeeded++
		}
	}
	if succeeded < 200 {
		t.Errorf("maps parsed without error = %d, want at least 200", succeeded)
	}
}

func TestParseMapRejectsGarbage(t *testing.T) {
	garbage := make([]byte, 100)
	for i := range garbage {
		garbage[i] = 0xFF
	}
	if _, err := ParseMap(garbage, 0); err == nil {
		t.Error("ParseMap(100-byte garbage) = nil, want error")
	}
}
