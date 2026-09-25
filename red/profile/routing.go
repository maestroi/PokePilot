package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/worldmodel"
)

const liveMapBorderBlocks = 3

func (*Profile) MapProvider(romData []byte) worldmodel.MapHeaderProvider {
	return redrom.NewWorldProvider(romData)
}

func (*Profile) DecodeLiveTopology(reader game.MemoryReader) (game.LiveTopologyState, error) {
	if reader == nil {
		return game.LiveTopologyState{}, fmt.Errorf("red profile: nil routing memory reader")
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])

	width := int(mem.U8(sym.CurMapWidth))
	height := int(mem.U8(sym.CurMapHeight))
	blocks, err := decodeLiveMapBlocks(&mem, width, height)
	if err != nil {
		return game.LiveTopologyState{}, err
	}
	mode := game.TraversalLand
	if mem.U8(sym.WalkBikeSurfState) == fieldSurfingState {
		mode = game.TraversalWater
	}

	live := state.DecodeSprites(&mem)
	liveObjects := make([]game.LiveMapObject, 0, len(live))
	for _, sprite := range live {
		liveObjects = append(liveObjects, game.LiveMapObject{
			Slot: sprite.Slot,
			X:    sprite.X,
			Y:    sprite.Y,
		})
	}

	positions := make(map[int]game.MapPoint)
	for slot, at := range state.DecodeObjectTiles(&mem) {
		positions[slot] = game.MapPoint{X: at[0], Y: at[1]}
	}
	hidden := make(map[int]bool)
	for slot := range state.HiddenObjectIDs(&mem) {
		hidden[int(slot)] = true
	}

	return game.LiveTopologyState{
		NativeMapID:     uint16(mem.U8(sym.CurMap)),
		WidthBlocks:     width,
		HeightBlocks:    height,
		Blocks:          blocks,
		Traversal:       mode,
		LiveObjects:     liveObjects,
		ObjectPositions: positions,
		HiddenObjects:   hidden,
	}, nil
}

func decodeLiveMapBlocks(mem *state.Mem, width, height int) ([]byte, error) {
	if width < 0 || height < 0 {
		return nil, fmt.Errorf("red profile: live map has negative dimensions %dx%d", width, height)
	}
	if width == 0 || height == 0 {
		return []byte{}, nil
	}
	stride := width + 2*liveMapBorderBlocks
	first := liveMapBorderBlocks*stride + liveMapBorderBlocks
	last := first + (height-1)*stride + (width - 1)
	if first < 0 || last >= sym.OverworldMapLen {
		return nil, fmt.Errorf("red profile: live map %dx%d needs wOverworldMap offset %d, buffer length is %d", width, height, last, sym.OverworldMapLen)
	}
	blocks := make([]byte, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			off := first + y*stride + x
			blocks[y*width+x] = mem.U8(sym.OverworldMap + uint16(off))
		}
	}
	return blocks, nil
}
