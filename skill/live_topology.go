package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const liveMapBorderBlocks = 3

// readLiveMapBlocks extracts the current map's interior block IDs from
// wOverworldMap. Pokémon Red stores a three-block connection border on every
// side, and ReplaceTileBlock writes directly into this buffer, so these are
// the block IDs the running game is actually navigating now.
func readLiveMapBlocks(peek func(uint16) uint8, widthBlocks, heightBlocks int) ([]byte, error) {
	if widthBlocks < 0 || heightBlocks < 0 {
		return nil, fmt.Errorf("skill: live map has negative dimensions %dx%d", widthBlocks, heightBlocks)
	}
	if widthBlocks == 0 || heightBlocks == 0 {
		return []byte{}, nil
	}

	stride := widthBlocks + 2*liveMapBorderBlocks
	first := liveMapBorderBlocks*stride + liveMapBorderBlocks
	last := first + (heightBlocks-1)*stride + (widthBlocks - 1)
	if first < 0 || last >= sym.OverworldMapLen {
		return nil, fmt.Errorf("skill: live map %dx%d needs wOverworldMap offset %d, buffer length is %d", widthBlocks, heightBlocks, last, sym.OverworldMapLen)
	}

	blocks := make([]byte, widthBlocks*heightBlocks)
	for y := 0; y < heightBlocks; y++ {
		for x := 0; x < widthBlocks; x++ {
			off := first + y*stride + x
			blocks[y*widthBlocks+x] = peek(sym.OverworldMap + uint16(off))
		}
	}
	return blocks, nil
}

func liveMapBlocks(m *emu.Emu, h rom.MapHeader) ([]byte, error) {
	widthBlocks, heightBlocks := int(h.WidthBlocks), int(h.HeightBlocks)
	if got := int(m.Peek8(sym.CurMapWidth)); got != widthBlocks {
		return nil, fmt.Errorf("skill: live map width is %d blocks, ROM header for map %02x says %d", got, h.ID, widthBlocks)
	}
	if got := int(m.Peek8(sym.CurMapHeight)); got != heightBlocks {
		return nil, fmt.Errorf("skill: live map height is %d blocks, ROM header for map %02x says %d", got, h.ID, heightBlocks)
	}
	return readLiveMapBlocks(m.Peek8, widthBlocks, heightBlocks)
}

// liveMapGrid decodes the current post-script map geometry using the same
// tileset collision rules as world.Build. It intentionally has no cache: every
// navigation call observes the current loaded map, including block replacements
// made since the ROM map data was loaded.
func liveMapGrid(m *emu.Emu, romData []byte, h rom.MapHeader) (*world.Grid, error) {
	blocks, err := liveMapBlocks(m, h)
	if err != nil {
		return nil, err
	}
	return world.BuildFromBlocks(romData, h, blocks)
}
