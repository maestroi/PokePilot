package gen1rom

import (
	"fmt"

	"github.com/maestroi/pokepilot/worldmodel"
)

type PairProvider func(rom []byte, tileset uint8, mode worldmodel.TraversalMode) map[[2]uint8]bool
type LedgeProvider func(rom []byte, tileset uint8) []worldmodel.Ledge

type GridLayout struct {
	TilesetsBank    uint8
	TilesetsAddr    uint16
	TilesetEntryLen int
	TilePairs       PairProvider
	Ledges          LedgeProvider
}

func BuildGridSpec(rom []byte, h MapHeader, blocks []byte, mode worldmodel.TraversalMode, layout GridLayout) (worldmodel.GridSpec, error) {
	width, height := int(h.WidthBlocks)*2, int(h.HeightBlocks)*2
	spec := worldmodel.GridSpec{
		MapID: h.ID, Width: width, Height: height,
		Walkable: make([]bool, width*height),
		CollisionTile: make([]uint8, width*height),
		FieldTile: make([]uint8, width*height),
		TilePairs: map[[2]uint8]bool{},
	}
	if layout.TilePairs != nil {
		spec.TilePairs = layout.TilePairs(rom, h.Tileset, mode)
	}
	if layout.Ledges != nil {
		spec.Ledges = append(spec.Ledges, layout.Ledges(rom, h.Tileset)...)
	}
	if width == 0 || height == 0 {
		return spec, nil
	}
	if layout.TilesetEntryLen <= 0 {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: invalid tileset entry length %d", h.ID, layout.TilesetEntryLen)
	}

	wantBlocks := int(h.WidthBlocks) * int(h.HeightBlocks)
	if blocks == nil {
		var err error
		blocks, err = Blocks(rom, h)
		if err != nil {
			return worldmodel.GridSpec{}, err
		}
	}
	if len(blocks) < wantBlocks {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: block map has %d bytes, want at least %d", h.ID, len(blocks), wantBlocks)
	}
	blocks = blocks[:wantBlocks]

	tsOff, err := BankedOffset(layout.TilesetsBank, layout.TilesetsAddr)
	if err != nil {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: %v", h.ID, err)
	}
	entryOff := tsOff + int(h.Tileset)*layout.TilesetEntryLen
	if entryOff+layout.TilesetEntryLen > len(rom) {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: tileset %d entry at offset %d exceeds ROM of %d bytes", h.ID, h.Tileset, entryOff, len(rom))
	}
	tsBank := rom[entryOff]
	blockPtr := uint16(rom[entryOff+1]) | uint16(rom[entryOff+2])<<8
	collPtr := uint16(rom[entryOff+5]) | uint16(rom[entryOff+6])<<8
	spec.CounterTiles = [3]uint8{rom[entryOff+7], rom[entryOff+8], rom[entryOff+9]}

	collBank := uint8(0)
	if collPtr >= 0x4000 {
		collBank = tsBank
	}
	collOff, err := BankedOffset(collBank, collPtr)
	if err != nil {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: %v", h.ID, err)
	}
	walkableTiles := make([]bool, 256)
	for {
		if collOff >= len(rom) {
			return worldmodel.GridSpec{}, fmt.Errorf("map %d: collision list at offset %d exceeds ROM of %d bytes", h.ID, collOff, len(rom))
		}
		t := rom[collOff]
		collOff++
		if t == 0xff {
			break
		}
		walkableTiles[t] = true
	}

	blockOff, err := BankedOffset(tsBank, blockPtr)
	if err != nil {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: %v", h.ID, err)
	}
	wb := int(h.WidthBlocks)
	for by := 0; by < int(h.HeightBlocks); by++ {
		for bx := 0; bx < wb; bx++ {
			blockID := blocks[by*wb+bx]
			tilesOff := blockOff + int(blockID)*16
			if tilesOff+16 > len(rom) {
				return worldmodel.GridSpec{}, fmt.Errorf("map %d: block %d data at offset %d exceeds ROM of %d bytes", h.ID, blockID, tilesOff, len(rom))
			}
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					field := rom[tilesOff+(2*sy)*4+2*sx]
					collision := rom[tilesOff+(2*sy+1)*4+2*sx]
					i := (by*2+sy)*width + (bx*2 + sx)
					spec.FieldTile[i] = field
					spec.CollisionTile[i] = collision
					spec.Walkable[i] = walkableTiles[collision]
				}
			}
		}
	}
	return spec, nil
}
