package controller

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowLiveMapBorderBlocks = 3
	yellowSurfWaterTile       = 0x14
)

func readYellowLiveMapBlocks(peek func(uint16) uint8, widthBlocks, heightBlocks int) ([]byte, error) {
	if widthBlocks < 0 || heightBlocks < 0 {
		return nil, fmt.Errorf("yellow travel: negative live map dimensions %dx%d", widthBlocks, heightBlocks)
	}
	if widthBlocks == 0 || heightBlocks == 0 {
		return []byte{}, nil
	}
	stride := widthBlocks + 2*yellowLiveMapBorderBlocks
	first := yellowLiveMapBorderBlocks*stride + yellowLiveMapBorderBlocks
	last := first + (heightBlocks-1)*stride + widthBlocks - 1
	if first < 0 || last >= sym.OverworldMapLen {
		return nil, fmt.Errorf("yellow travel: live map %dx%d needs wOverworldMap offset %d, buffer=%d",
			widthBlocks, heightBlocks, last, sym.OverworldMapLen)
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

func yellowLiveMapBlocks(m *emu.Emu, h yellowrom.MapHeader) ([]byte, error) {
	width, height := int(h.WidthBlocks), int(h.HeightBlocks)
	if got := int(m.Peek8(sym.CurMapWidth)); got != width {
		return nil, fmt.Errorf("yellow travel: live width=%d, ROM map %#02x width=%d", got, h.ID, width)
	}
	if got := int(m.Peek8(sym.CurMapHeight)); got != height {
		return nil, fmt.Errorf("yellow travel: live height=%d, ROM map %#02x height=%d", got, h.ID, height)
	}
	return readYellowLiveMapBlocks(m.Peek8, width, height)
}

func yellowLiveMapGridForTraversal(m *emu.Emu, romData []byte, h yellowrom.MapHeader, mode world.TraversalMode) (*world.Grid, error) {
	blocks, err := yellowLiveMapBlocks(m, h)
	if err != nil {
		return nil, err
	}
	grid, err := world.BuildFromBlocksForTraversal(romData, h, blocks, mode)
	if err != nil {
		return nil, err
	}
	if mode == world.TraversalWater {
		for y := 0; y < grid.Height; y++ {
			for x := 0; x < grid.Width; x++ {
				field, fieldOK := grid.FieldTile(x, y)
				collision, collisionOK := grid.Tile(x, y)
				if (fieldOK && field == yellowSurfWaterTile) || (collisionOK && collision == yellowSurfWaterTile) {
					grid.Set(x, y, true)
				}
			}
		}
	}
	return grid, nil
}

func yellowLiveMapGrid(m *emu.Emu, romData []byte, h yellowrom.MapHeader) (*world.Grid, error) {
	mode := world.TraversalLand
	if m.Peek8(sym.WalkBikeSurfState) == 2 {
		mode = world.TraversalWater
	}
	return yellowLiveMapGridForTraversal(m, romData, h, mode)
}
