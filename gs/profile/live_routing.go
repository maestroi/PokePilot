package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

const gen2LiveMapBorderBlocks = 3

func decodeGen2LiveMapBlocks(reader game.MemoryReader, width, height int) ([]byte, error) {
	if reader == nil {
		return nil, fmt.Errorf("gs profile: nil routing memory reader")
	}
	if width < 0 || height < 0 {
		return nil, fmt.Errorf("gs profile: live map has negative dimensions %dx%d", width, height)
	}
	if width == 0 || height == 0 {
		return []byte{}, nil
	}
	stride := width + 2*gen2LiveMapBorderBlocks
	first := gen2LiveMapBorderBlocks*stride + gen2LiveMapBorderBlocks
	last := first + (height-1)*stride + width - 1
	if last >= sym.OverworldMapLen {
		return nil, fmt.Errorf("gs profile: live map %dx%d needs wOverworldMap offset %d, buffer length is %d", width, height, last, sym.OverworldMapLen)
	}
	out := make([]byte, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out[y*width+x] = reader.Peek8(sym.OverworldMap + uint16(first+y*stride+x))
		}
	}
	return out, nil
}

func gen2Traversal(reader game.MemoryReader) game.TraversalMode {
	switch reader.Peek8(sym.PlayerState) {
	case 4, 8: // PLAYER_SURF, PLAYER_SURF_PIKA
		return game.TraversalWater
	default:
		return game.TraversalLand
	}
}

func decodeGen2LiveObjects(reader game.MemoryReader) ([]game.LiveMapObject, map[int]game.MapPoint) {
	objects := make([]game.LiveMapObject, 0, sym.NumObjectStructs-1)
	positions := make(map[int]game.MapPoint)
	// Object struct 0 is the player. Active NPC structs occupy 1..12 and
	// retain their 1-based map-object identity at offset 1.
	for i := 1; i < sym.NumObjectStructs; i++ {
		base := sym.ObjectStructs + uint16(i*sym.ObjectStructLen)
		if reader.Peek8(base) == 0 {
			continue
		}
		slot := int(reader.Peek8(base + 1))
		rawX, rawY := reader.Peek8(base+0x10), reader.Peek8(base+0x11)
		if rawX < 4 || rawY < 4 || slot <= 0 {
			continue
		}
		x, y := int(rawX)-4, int(rawY)-4
		objects = append(objects, game.LiveMapObject{Slot: slot, X: x, Y: y})
		positions[slot] = game.MapPoint{X: x, Y: y}
	}
	return objects, positions
}

// DecodeLiveTopology exposes the mutable half of Gen-II routing: current map
// blocks, traversal mode and active object positions. Static warps/connections
// remain owned by the native world provider.
func (*Profile) DecodeLiveTopology(reader game.MemoryReader) (game.LiveTopologyState, error) {
	if reader == nil {
		return game.LiveTopologyState{}, fmt.Errorf("gs profile: nil routing memory reader")
	}
	width, height := int(reader.Peek8(sym.MapWidth)), int(reader.Peek8(sym.MapHeight))
	blocks, err := decodeGen2LiveMapBlocks(reader, width, height)
	if err != nil {
		return game.LiveTopologyState{}, err
	}
	objects, positions := decodeGen2LiveObjects(reader)
	return game.LiveTopologyState{
		NativeMapID: gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber)),
		WidthBlocks: width,
		HeightBlocks: height,
		Blocks: blocks,
		Traversal: gen2Traversal(reader),
		LiveObjects: objects,
		ObjectPositions: positions,
		HiddenObjects: map[int]bool{},
	}, nil
}
