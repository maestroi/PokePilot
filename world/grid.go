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
// the same coordinates reported by wXCoord/wYCoord. Each game step covers a
// 2x2 pair of background tiles and the ROM deliberately uses DIFFERENT
// subtiles for two jobs:
//
//   - collision uses the bottom-left tile (row 2*sy+1, column 2*sx), the
//     measured rule this package has always used;
//   - GetTileAndCoordsInFrontOfPlayer uses the top-left tile (row 2*sy,
//     column 2*sx), which is the value field actions such as CUT compare.
//
// Both ids are retained so callers never infer one contract from the other.
type Grid struct {
	MapID         uint8
	Width, Height int // in game tile coordinates
	walkable      []bool
	collisionTile []uint8
	fieldTile     []uint8
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

// Tile returns the bottom-left collision tile id for a game-tile coordinate.
func (g *Grid) Tile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) || len(g.collisionTile) != g.Width*g.Height {
		return 0, false
	}
	return g.collisionTile[y*g.Width+x], true
}

// FieldTile returns the top-left background tile id that
// GetTileAndCoordsInFrontOfPlayer exposes through wTileInFrontOfPlayer when
// this game-coordinate cell is directly in front of the player.
func (g *Grid) FieldTile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) || len(g.fieldTile) != g.Width*g.Height {
		return 0, false
	}
	return g.fieldTile[y*g.Width+x], true
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
		MapID:         h.ID,
		Width:         width,
		Height:        height,
		walkable:      make([]bool, width*height),
		collisionTile: make([]uint8, width*height),
		fieldTile:     make([]uint8, width*height),
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
			// Each block is 4x4 background tiles; one game-coordinate cell is
			// the corresponding 2x2 background-tile pair. Collision and field
			// actions intentionally read different left-hand subtiles.
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					field := romData[tilesOff+(2*sy)*4+2*sx]
					collision := romData[tilesOff+(2*sy+1)*4+2*sx]
					i := (by*2+sy)*width + (bx*2 + sx)
					g.fieldTile[i] = field
					g.collisionTile[i] = collision
					g.walkable[i] = walkableTiles[collision]
				}
			}
		}
	}

	return g, nil
}
