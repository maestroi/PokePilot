// Package worldmap adapts Pokémon Red ROM topology and collision data to the
// game-agnostic world package.
package worldmap

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// Red tileset/table layout. These are adapter facts and must not leak into
// generic world routing.
const (
	tilesetsBank    uint8  = 0x03
	tilesetsAddr    uint16 = 0x47BE
	tilesetEntryLen        = 12

	tilePairCollisionsLandAddr  = 0x0c7e
	tilePairCollisionsWaterAddr = 0x0ca0
	tilePairEntryLen            = 3
)

func bankedOffset(bank uint8, addr uint16) (int, error) {
	if addr >= 0x4000 {
		return int(bank)*0x4000 + int(addr-0x4000), nil
	}
	if bank != 0 {
		return 0, fmt.Errorf("address %04X in bank %d is below 0x4000", addr, bank)
	}
	return int(addr), nil
}

func tilePairsForTraversal(romData []byte, tileset uint8, mode world.TraversalMode) map[[2]uint8]bool {
	pairs := map[[2]uint8]bool{}
	addr := tilePairCollisionsLandAddr
	if mode == world.TraversalWater {
		addr = tilePairCollisionsWaterAddr
	}
	for off := addr; off+tilePairEntryLen <= len(romData); off += tilePairEntryLen {
		if romData[off] == 0xff {
			break
		}
		if romData[off] != tileset {
			continue
		}
		a, b := romData[off+1], romData[off+2]
		pairs[[2]uint8{a, b}] = true
		pairs[[2]uint8{b, a}] = true
	}
	return pairs
}

// Build constructs a collision grid for a parsed Red map from its immutable
// ROM block map.
func Build(romData []byte, h rom.MapHeader) (*world.Grid, error) {
	if h.WidthBlocks == 0 || h.HeightBlocks == 0 {
		return BuildFromBlocks(romData, h, nil)
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		return nil, err
	}
	return BuildFromBlocks(romData, h, blocks)
}

// BuildFromBlocks constructs a Red collision grid from live or immutable block
// ids using land movement rules.
func BuildFromBlocks(romData []byte, h rom.MapHeader, blocks []byte) (*world.Grid, error) {
	return BuildFromBlocksForTraversal(romData, h, blocks, world.TraversalLand)
}

// BuildFromBlocksForTraversal decodes Red's movement-mode-specific tile-pair
// collision rules and translates them to a generic world.Grid.
func BuildFromBlocksForTraversal(romData []byte, h rom.MapHeader, blocks []byte, mode world.TraversalMode) (*world.Grid, error) {
	width := int(h.WidthBlocks) * 2
	height := int(h.HeightBlocks) * 2
	spec := world.GridSpec{
		MapID:         h.ID,
		Width:         width,
		Height:        height,
		Walkable:      make([]bool, width*height),
		CollisionTile: make([]uint8, width*height),
		FieldTile:     make([]uint8, width*height),
		TilePairs:     tilePairsForTraversal(romData, h.Tileset, mode),
	}
	for _, l := range rom.Ledges(romData, h.Tileset) {
		spec.Ledges = append(spec.Ledges, world.Ledge{DX: l.DX, DY: l.DY, From: l.From, Over: l.Over})
	}
	if width == 0 || height == 0 {
		return world.NewGrid(spec)
	}

	wantBlocks := int(h.WidthBlocks) * int(h.HeightBlocks)
	if len(blocks) < wantBlocks {
		return nil, fmt.Errorf("map %d: block map has %d bytes, want at least %d", h.ID, len(blocks), wantBlocks)
	}
	blocks = blocks[:wantBlocks]

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
	spec.CounterTiles = [3]uint8{romData[entryOff+7], romData[entryOff+8], romData[entryOff+9]}

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
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					field := romData[tilesOff+(2*sy)*4+2*sx]
					collision := romData[tilesOff+(2*sy+1)*4+2*sx]
					i := (by*2+sy)*width + (bx*2 + sx)
					spec.FieldTile[i] = field
					spec.CollisionTile[i] = collision
					spec.Walkable[i] = walkableTiles[collision]
				}
			}
		}
	}

	return world.NewGrid(spec)
}
