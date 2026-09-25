package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

const (
	liveMapBorderBlocks = 3
	// surfWaterTile is retained for Gen-I compatibility callers that inspect
	// decoded tile ids directly. Generic routing derives water traversal from
	// the adapter's land/water grid semantics.
	surfWaterTile uint8 = 0x14
)

// readLiveMapBlocks is retained for Gen-I compatibility tests and observation
// callers that still hold a red/state snapshot. Production routing uses the
// active profile's game.RoutingDecoder instead.
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

// LiveMapGridFromMem preserves the legacy Red snapshot API for observation
// code. New game-generic routing should use liveMapGrid/liveMapGridWithRuntime.
func LiveMapGridFromMem(mem *state.Mem, romData []byte, h worldmodel.HeaderView) (*world.Grid, error) {
	if mem == nil {
		return nil, fmt.Errorf("skill: live map grid: nil memory snapshot")
	}
	return liveMapGridFromMem(mem, romData, h)
}

func liveMapGridFromMem(mem *state.Mem, romData []byte, h worldmodel.HeaderView) (*world.Grid, error) {
	header := h.WorldMapHeader()
	widthBlocks, heightBlocks := int(header.WidthBlocks), int(header.HeightBlocks)
	if got := int(mem.U8(sym.CurMapWidth)); got != widthBlocks {
		return nil, fmt.Errorf("skill: live map width is %d blocks, ROM header for map %02x says %d", got, header.ID, widthBlocks)
	}
	if got := int(mem.U8(sym.CurMapHeight)); got != heightBlocks {
		return nil, fmt.Errorf("skill: live map height is %d blocks, ROM header for map %02x says %d", got, header.ID, heightBlocks)
	}
	blocks, err := readLiveMapBlocks(mem.U8, widthBlocks, heightBlocks)
	if err != nil {
		return nil, err
	}
	mode := world.TraversalLand
	if mem.U8(sym.WalkBikeSurfState) == fieldSurfingState {
		mode = world.TraversalWater
	}
	return buildLiveMapGrid(romData, h, blocks, mode)
}
