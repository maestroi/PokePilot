package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	liveMapBorderBlocks = 3
	// IsNextTileShoreOrWater in the Red ROM accepts $14 as the ordinary
	// water tile. Land collision lists intentionally exclude it, so a Surf
	// traversal grid must add it back after decoding the shared map blocks.
	surfWaterTile uint8 = 0x14
)

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

// liveMapGrid decodes the current post-script geometry using the traversal mode
// the game is actually in. It intentionally has no cache: semantic transitions
// such as Cut and Surf are followed by a fresh decode before routing continues.
func liveMapGrid(m *emu.Emu, romData []byte, h rom.MapHeader) (*world.Grid, error) {
	mode := world.TraversalLand
	if m.Peek8(sym.WalkBikeSurfState) == fieldSurfingState {
		mode = world.TraversalWater
	}
	return liveMapGridForTraversal(m, romData, h, mode)
}

func liveMapGridForTraversal(m *emu.Emu, romData []byte, h rom.MapHeader, mode world.TraversalMode) (*world.Grid, error) {
	blocks, err := liveMapBlocks(m, h)
	if err != nil {
		return nil, err
	}
	grid, err := world.BuildFromBlocksForTraversal(romData, h, blocks, mode)
	if err != nil {
		return nil, err
	}
	if mode == world.TraversalWater {
		// BuildFromBlocksForTraversal swaps the ROM's tile-pair collision table,
		// but the base collision list is shared with land movement and does not
		// itself include water. The actual game permits $14 while surfing, so
		// project that same semantic fact into live pathfinding. Checking both
		// top-left field-action and bottom-left collision subtiles covers the
		// two tile contracts the decoder deliberately keeps separate.
		for y := 0; y < grid.Height; y++ {
			for x := 0; x < grid.Width; x++ {
				field, fieldOK := grid.FieldTile(x, y)
				collision, collisionOK := grid.Tile(x, y)
				if (fieldOK && field == surfWaterTile) || (collisionOK && collision == surfWaterTile) {
					grid.Set(x, y, true)
				}
			}
		}
	}
	return grid, nil
}
