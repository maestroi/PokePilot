package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
)

// Tileset table layout (pokered.sym: Tilesets = 03:47BE), 12 bytes per entry:
//
//	+0  bank of the tileset's block/gfx data
//	+1  block pointer (2 bytes, little-endian)
//	+3  gfx pointer (2 bytes)
//	+5  collision pointer (2 bytes) -> walkable tile ids, 0xff terminated
//	+7  counter tiles (3 bytes)
//	+10 grass tile
//	+11 animation
const (
	tilesetsBank    uint8  = 0x03
	tilesetsAddr    uint16 = 0x47BE
	tilesetEntryLen        = 12
)

// Grid is a map's collision view, indexed [y][x] in game tile coordinates —
// the same coordinates reported by wXCoord/wYCoord. Besides walkability it
// retains the collision tile id for each cell. That matters for field moves:
// a solid CUT tree and an ordinary solid wall are both non-walkable, but only
// the former is a legal obstacle for navigation to remove.
type Grid struct {
	MapID         uint8
	Width, Height int // in game tile coordinates
	walkable      []bool
	tiles         []uint8
}

// InBounds reports whether (x, y) is inside the grid.
func (g *Grid) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.Width && y < g.Height
}

// Walkable reports whether the game-tile coordinate (x, y) is walkable.
// Out-of-bounds coordinates are not walkable.
func (g *Grid) Walkable(x, y int) bool {
	if !g.InBounds(x, y) {
		return false
	}
	return g.walkable[y*g.Width+x]
}

// Tile returns the collision tile id for a game-tile coordinate. The value is
// the same bottom-left tile that Build uses for collision and that the game
// compares with wTileInFrontOfPlayer for field moves such as CUT.
func (g *Grid) Tile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) || len(g.tiles) != g.Width*g.Height {
		return 0, false
	}
	return g.tiles[y*g.Width+x], true
}

// Set sets the walkability of the game-tile coordinate (x, y).
// Out-of-bounds coordinates are ignored.
func (g *Grid) Set(x, y int, ok bool) {
	if !g.InBounds(x, y) {
		return
	}
	g.walkable[y*g.Width+x] = ok
}

// bankedOffset converts a banked address (bank:addr) to a ROM file offset.
func bankedOffset(bank uint8, addr uint16) (int, error) {
	if addr >= 0x4000 {
		return int(bank)*0x4000 + int(addr-0x4000), nil
	}
	if bank != 0 {
		return 0, fmt.Errorf("address %04X in bank %d is below 0x4000", addr, bank)
	}
	return int(addr), nil
}

// Build constructs the collision grid for a parsed map.
func Build(romData []byte, h rom.MapHeader) (*Grid, error) {
	width := int(h.WidthBlocks) * 2
	height := int(h.HeightBlocks) * 2
	g := &Grid{
		MapID:    h.ID,
		Width:    width,
		Height:   height,
		walkable: make([]bool, width*height),
		tiles:    make([]uint8, width*height),
	}
	if width == 0 || height == 0 {
		return g, nil
	}

	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		return nil, err
	}

	// Look up the map's tileset entry.
	tsOff, err := bankedOffset(tilesetsBank, tilesetsAddr)
	if err != nil {
		return nil, fmt.Errorf("map %d: %v", h.ID, err)
	}
	entryOff := tsOff + int(h.Tileset)*tilesetEntryLen
	if entryOff+tilesetEntryLen > len(romData) {
		return nil, fmt.Errorf("map %d: tileset %d entry at offset %d exceeds ROM of %d bytes", h.ID, h.Tileset, entryOff, len(romData))
	}
	tsBank := romData[entryOff]
	blockPtr := uint16(romData[entryOff+1]) | uint16(romData[entryOff+2])<<8
	collPtr := uint16(romData[entryOff+5]) | uint16(romData[entryOff+6])<<8

	// Read the tileset's walkable-tile list. The list lives in bank 0 (the
	// Home section); the game dereferences it with no bank switch.
	collBank := uint8(0)
	if collPtr >= 0x4000 {
		collBank = tsBank
	}
	collOff, err := bankedOffset(collBank, collPtr)
	if err != nil {
		return nil, fmt.Errorf("map %d: %v", h.ID, err)
	}
	walkableTiles := make([]bool, 256)
	for {
		if collOff >= len(romData) {
			return nil, fmt.Errorf("map %d: collision list at offset %d exceeds ROM of %d bytes", h.ID, collOff, len(romData))
		}
		t := romData[collOff]
		collOff++
		if t == 0xff {
			break
		}
		walkableTiles[t] = true
	}

	// Read the block definitions: 16 tile ids per block, in the tileset bank.
	blockOff, err := bankedOffset(tsBank, blockPtr)
	if err != nil {
		return nil, fmt.Errorf("map %d: %v", h.ID, err)
	}

	wb := int(h.WidthBlocks)
	for by := 0; by < int(h.HeightBlocks); by++ {
		for bx := 0; bx < wb; bx++ {
			blockID := blocks[by*wb+bx]
			tilesOff := blockOff + int(blockID)*16
			if tilesOff+16 > len(romData) {
				return nil, fmt.Errorf("map %d: block %d data at offset %d exceeds ROM of %d bytes", h.ID, blockID, tilesOff, len(romData))
			}
			// Each block is 4x4 tiles; the game's coordinates are 2x2-tile
			// steps, so a block covers 2x2 grid cells. The collision tile
			// is the one the player stands on: the step's bottom-left
			// (block tile row 2*sy+1, column 2*sx). Measured against
			// Oak's Lab (map 0x28), not assumed.
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					tile := romData[tilesOff+(2*sy+1)*4+2*sx]
					i := (by*2+sy)*width + (bx*2 + sx)
					g.tiles[i] = tile
					g.walkable[i] = walkableTiles[tile]
				}
			}
		}
	}

	return g, nil
}
