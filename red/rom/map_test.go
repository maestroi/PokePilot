package rom

import (
	"os"
	"testing"
)

// Map ids from constants/map_constants.asm.
const (
	mapPalletTown     uint8 = 0x00
	mapRoute1         uint8 = 0x0C
	mapRoute21        uint8 = 0x20
	mapRedsHouse1F    uint8 = 0x25
	mapBluesHouse     uint8 = 0x27
	mapOaksLab        uint8 = 0x28
	mapViridianForest uint8 = 0x33
	mapPewterGym      uint8 = 0x36
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

// findObject returns the object at (x, y) on the map, if any.
func findObject(h MapHeader, x, y uint8) (*Object, bool) {
	for i := range h.Objects {
		if h.Objects[i].X == x && h.Objects[i].Y == y {
			return &h.Objects[i], true
		}
	}
	return nil, false
}

func TestParseViridianForestItems(t *testing.T) {
	r := loadROM(t)
	h, err := ParseMap(r, mapViridianForest)
	if err != nil {
		t.Fatalf("ParseMap(ViridianForest) = %v, want nil", err)
	}

	var items []Object
	for _, o := range h.Objects {
		if o.TextID&0x80 != 0 {
			items = append(items, o)
		}
	}
	want := []struct {
		x, y uint8
		id   uint8
	}{
		{25, 11, 0},   // ANTIDOTE, id not asserted
		{12, 29, 0},   // POTION, id not asserted
		{1, 31, 0x04}, // POKE_BALL
	}
	if len(items) != len(want) {
		t.Fatalf("item objects = %d, want %d: %#v", len(items), len(want), items)
	}
	for i, w := range want {
		o := items[i]
		if o.X != w.x || o.Y != w.y {
			t.Errorf("items[%d] at (%d,%d), want (%d,%d)", i, o.X, o.Y, w.x, w.y)
		}
		if o.SpriteID != 0x3D { // SPRITE_POKE_BALL
			t.Errorf("items[%d] SpriteID = %#x, want 0x3D", i, o.SpriteID)
		}
		if w.id != 0 && o.ItemID != w.id {
			t.Errorf("items[%d] ItemID = %#x, want %#x", i, o.ItemID, w.id)
		}
		if o.TrainerClass != 0 || o.TrainerSet != 0 {
			t.Errorf("items[%d] TrainerClass/TrainerSet = %#x/%#x, want 0/0 (no cross-contamination)", i, o.TrainerClass, o.TrainerSet)
		}
	}

	// A plain NPC on the same map: ItemID must be 0.
	if npc, ok := findObject(h, 16, 43); !ok {
		t.Fatal("no object at (16,43), want a plain NPC")
	} else if npc.ItemID != 0 || npc.TrainerClass != 0 || npc.TrainerSet != 0 {
		t.Errorf("plain NPC at (16,43) = %#v, want ItemID/TrainerClass/TrainerSet all 0", npc)
	}
}

func TestParsePewterGymTrainers(t *testing.T) {
	r := loadROM(t)
	h, err := ParseMap(r, mapPewterGym)
	if err != nil {
		t.Fatalf("ParseMap(PewterGym) = %v, want nil", err)
	}

	var trainers, plain []Object
	for _, o := range h.Objects {
		switch {
		case o.TextID&0x40 != 0:
			trainers = append(trainers, o)
		case o.TextID&0x80 == 0:
			plain = append(plain, o)
		}
	}

	if len(trainers) != 2 {
		t.Fatalf("trainer objects = %d, want 2: %#v", len(trainers), trainers)
	}
	for _, tc := range []struct{ x, y uint8 }{{4, 1}, {3, 6}} {
		o, ok := findObject(h, tc.x, tc.y)
		if !ok || o.TextID&0x40 == 0 {
			t.Errorf("no trainer at (%d,%d): %#v", tc.x, tc.y, o)
			continue
		}
		if o.ItemID != 0 {
			t.Errorf("trainer at (%d,%d) ItemID = %#x, want 0 (no cross-contamination)", tc.x, tc.y, o.ItemID)
		}
		if o.TrainerClass == 0 || o.TrainerSet == 0 {
			t.Errorf("trainer at (%d,%d) TrainerClass/TrainerSet = %#x/%#x, want nonzero", tc.x, tc.y, o.TrainerClass, o.TrainerSet)
		}
	}

	if len(plain) != 1 {
		t.Fatalf("plain NPC objects = %d, want 1: %#v", len(plain), plain)
	}
	guide := plain[0]
	if guide.X != 7 || guide.Y != 10 {
		t.Errorf("plain NPC at (%d,%d), want (7,10) the gym guide", guide.X, guide.Y)
	}
	if guide.ItemID != 0 || guide.TrainerClass != 0 || guide.TrainerSet != 0 {
		t.Errorf("gym guide = %#v, want ItemID/TrainerClass/TrainerSet all 0", guide)
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
