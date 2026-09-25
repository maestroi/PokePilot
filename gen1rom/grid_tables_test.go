package gen1rom

import (
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func TestTilePairsAtFiltersTilesetAndIsSymmetric(t *testing.T) {
	rom := []byte{0x00, 0x11, 3, 0x14, 0x2e, 17, 0x20, 0x05, 0xff, 3, 0x99, 0x98}
	pairs := TilePairsAt(rom, 2, 3)
	if !pairs[[2]uint8{0x14, 0x2e}] || !pairs[[2]uint8{0x2e, 0x14}] {
		t.Fatalf("pairs = %v, want symmetric 14/2e", pairs)
	}
	if len(pairs) != 2 {
		t.Fatalf("pairs = %v; other tilesets or entries after $FF leaked in", pairs)
	}
}

func TestLedgesAtDecodesDirectionsOnlyOnOverworld(t *testing.T) {
	rom := []byte{
		0x00, 0x2c, 0x37, 0x80,
		0x08, 0x2c, 0x27, 0x20,
		0x0c, 0x39, 0x0d, 0x10,
		0xff,
	}
	got := LedgesAt(rom, 0, 0)
	want := []worldmodel.Ledge{
		{DY: 1, From: 0x2c, Over: 0x37},
		{DX: -1, From: 0x2c, Over: 0x27},
		{DX: 1, From: 0x39, Over: 0x0d},
	}
	if len(got) != len(want) {
		t.Fatalf("ledges = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ledge %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if got := LedgesAt(rom, 0, 1); got != nil {
		t.Fatalf("non-overworld tileset ledges = %+v, want none", got)
	}
}

func TestBuildGridSpecReadsSwitchableCollisionListFromLayoutBank(t *testing.T) {
	rom := make([]byte, 3*0x4000)
	// Tileset header table in bank 2 at $4000: GFX bank 2, block ptr $4100,
	// collision ptr $4200 (switchable). The collision list lives in bank 1.
	ts := 2*0x4000 + 0x0000
	rom[ts] = 2
	rom[ts+1], rom[ts+2] = 0x00, 0x41
	rom[ts+5], rom[ts+6] = 0x00, 0x42
	// Bank 1 collision list: tile $07 walkable.
	rom[1*0x4000+0x0200], rom[1*0x4000+0x0201] = 0x07, 0xff
	// Bank 2 at the same pointer says tile $09: wrong bank must not be used.
	rom[2*0x4000+0x0200], rom[2*0x4000+0x0201] = 0x09, 0xff
	// Block 0 in bank 2: collision tiles (odd rows) all $07.
	for i := 0; i < 16; i++ {
		rom[2*0x4000+0x0100+i] = 0x07
	}
	h := MapHeader{ID: 1, WidthBlocks: 1, HeightBlocks: 1}
	layout := GridLayout{TilesetsBank: 2, TilesetsAddr: 0x4000, TilesetEntryLen: 12, CollisionBank: 1}
	spec, err := BuildGridSpec(rom, h, []byte{0}, worldmodel.TraversalLand, layout)
	if err != nil {
		t.Fatal(err)
	}
	for i, ok := range spec.Walkable {
		if !ok {
			t.Fatalf("tile %d not walkable; collision list read from the GFX bank", i)
		}
	}
}
