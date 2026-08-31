package skill_test

// Throwaway measurement (S10-2 follow-up): is the (6,23) pocket on map 0x01
// sealed in the ROM's own collision data, or does world.Build misread a
// boundary step? Dumps the objects in the region and every sub-tile of each
// boundary step. Delete before commit.

import (
	"fmt"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

func TestZZPocket(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	h, err := rom.ParseMap(romData, 0x01)
	if err != nil {
		t.Fatal(err)
	}

	for i, o := range h.Objects {
		if o.X >= 2 && o.X <= 10 && o.Y >= 19 && o.Y <= 28 {
			fmt.Printf("object %d: home (%d,%d) sprite=%#04x move=%#04x range=%#04x text=%#04x item=%#04x\n",
				i+1, o.X, o.Y, o.SpriteID, o.Movement, o.Range, o.TextID, o.ItemID)
		}
	}

	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatal(err)
	}

	// Tileset table (world/grid.go): bank 0x03, addr 0x47BE, 12-byte entries.
	tsOff := int(0x03)*0x4000 + int(0x47BE-0x4000)
	entry := tsOff + int(h.Tileset)*12
	tsBank := romData[entry]
	blockPtr := uint16(romData[entry+1]) | uint16(romData[entry+2])<<8
	collPtr := uint16(romData[entry+5]) | uint16(romData[entry+6])<<8
	collBank := uint8(0)
	if collPtr >= 0x4000 {
		collBank = tsBank
	}
	var collOff int
	if collPtr >= 0x4000 {
		collOff = int(collBank)*0x4000 + int(collPtr-0x4000)
	} else {
		collOff = int(collPtr)
	}
	walkableTiles := make([]bool, 256)
	for {
		tid := romData[collOff]
		collOff++
		if tid == 0xff {
			break
		}
		walkableTiles[tid] = true
	}

	blockOff := int(tsBank)*0x4000 + int(blockPtr-0x4000)
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		t.Fatal(err)
	}
	wb := int(h.WidthBlocks)

	// stepTiles returns the 16 sub-tile ids of one block (2x2 steps).
	stepTiles := func(bx, by int) []uint8 {
		id := blocks[by*wb+bx]
		off := blockOff + int(id)*16
		tiles := make([]uint8, 16)
		copy(tiles, romData[off:off+16])
		return tiles
	}

	fmt.Println("region x=2..10 y=19..28; grid: W=walkable w=wall; mixed steps marked [M]")
	for y := 19; y <= 28; y++ {
		row := fmt.Sprintf("y=%2d ", y)
		for x := 2; x <= 10; x++ {
			bx, by := x/2, y/2
			sx, sy := x%2, y%2
			tiles := stepTiles(bx, by)
			// The step's own four sub-tiles: rows 2*sy..2*sy+1, cols 2*sx..2*sx+1.
			sub := []uint8{
				tiles[(2*sy)*4+(2*sx)], tiles[(2*sy)*4+(2*sx)+1],
				tiles[(2*sy+1)*4+(2*sx)], tiles[(2*sy+1)*4+(2*sx)+1],
			}
			collID := sub[3] // bottom-left: the single-sub-tile rule's pick
			mixed := false
			for _, t := range sub {
				if walkableTiles[t] != walkableTiles[collID] {
					mixed = true
				}
			}
			ch := "w"
			if grid.Walkable(x, y) {
				ch = "W"
			}
			if mixed {
				row += fmt.Sprintf("%s[%#02x M] ", ch, collID)
			} else {
				row += fmt.Sprintf("%s[%#02x]   ", ch, collID)
			}
		}
		fmt.Println(row)
	}
}
