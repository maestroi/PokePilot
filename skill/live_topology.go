package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

func liveTopologyState(m *emu.Emu) (game.LiveTopologyState, error) {
	routing, err := routingProfileFor(m)
	if err != nil {
		return game.LiveTopologyState{}, err
	}
	return routing.DecodeLiveTopology(m)
}

// liveMapBlocks returns the active profile's mutable row-major block map for
// the current map and verifies it matches the static provider header.
func liveMapBlocks(m *emu.Emu, h worldmodel.HeaderView) ([]byte, error) {
	routing, err := routingProfileFor(m)
	if err != nil {
		return nil, err
	}
	return liveMapBlocksWithDecoder(m, routing, h)
}

func liveMapBlocksWithDecoder(reader game.MemoryReader, decoder game.RoutingDecoder, h worldmodel.HeaderView) ([]byte, error) {
	if decoder == nil {
		return nil, fmt.Errorf("skill: live map: nil routing decoder")
	}
	header := h.WorldMapHeader()
	live, err := decoder.DecodeLiveTopology(reader)
	if err != nil {
		return nil, err
	}
	if live.NativeMapID != uint16(header.ID) {
		return nil, fmt.Errorf("skill: live map id is %02x, header is %02x", live.NativeMapID, header.ID)
	}
	if live.WidthBlocks != int(header.WidthBlocks) {
		return nil, fmt.Errorf("skill: live map width is %d blocks, ROM header for map %02x says %d", live.WidthBlocks, header.ID, header.WidthBlocks)
	}
	if live.HeightBlocks != int(header.HeightBlocks) {
		return nil, fmt.Errorf("skill: live map height is %d blocks, ROM header for map %02x says %d", live.HeightBlocks, header.ID, header.HeightBlocks)
	}
	want := live.WidthBlocks * live.HeightBlocks
	if len(live.Blocks) != want {
		return nil, fmt.Errorf("skill: live map %02x block payload has %d entries, want %d", header.ID, len(live.Blocks), want)
	}
	return append([]byte(nil), live.Blocks...), nil
}

// liveMapGrid decodes current post-script geometry using the traversal mode the
// active profile reports. Static collision interpretation stays in the ROM
// provider; skill never reads a concrete game's ROM tables.
func liveMapGrid(m *emu.Emu, romData []byte, h worldmodel.HeaderView) (*world.Grid, error) {
	routing, err := routingProfileFor(m)
	if err != nil {
		return nil, err
	}
	live, err := routing.DecodeLiveTopology(m)
	if err != nil {
		return nil, err
	}
	return liveMapGridWithRuntime(m, routing, routing.MapProvider(romData), h, world.TraversalMode(live.Traversal))
}

func liveMapGridForTraversal(m *emu.Emu, romData []byte, h worldmodel.HeaderView, mode world.TraversalMode) (*world.Grid, error) {
	routing, err := routingProfileFor(m)
	if err != nil {
		return nil, err
	}
	return liveMapGridWithRuntime(m, routing, routing.MapProvider(romData), h, mode)
}

func liveMapGridWithRuntime(
	reader game.MemoryReader,
	decoder game.RoutingDecoder,
	provider worldmodel.MapHeaderProvider,
	h worldmodel.HeaderView,
	mode world.TraversalMode,
) (*world.Grid, error) {
	if provider == nil {
		return nil, fmt.Errorf("skill: live map grid: nil map provider")
	}
	header := h.WorldMapHeader()
	blocks, err := liveMapBlocksWithDecoder(reader, decoder, header)
	if err != nil {
		return nil, err
	}
	spec, err := provider.Grid(header.ID, blocks, mode)
	if err != nil {
		return nil, err
	}
	return world.GridFromSpec(spec)
}

func buildLiveMapGrid(romData []byte, h worldmodel.HeaderView, blocks []byte, mode world.TraversalMode) (*world.Grid, error) {
	if gridHeader, ok := h.(worldmodel.GridHeader); ok {
		spec, err := gridHeader.WorldGridSpec(romData, blocks, mode)
		if err != nil {
			return nil, err
		}
		return world.GridFromSpec(spec)
	}
	provider, ok := worldmodel.ProviderForROM(romData)
	if !ok || provider == nil {
		return nil, fmt.Errorf("skill: live map grid: no map provider for ROM")
	}
	spec, err := provider.Grid(h.WorldMapHeader().ID, blocks, mode)
	if err != nil {
		return nil, err
	}
	return world.GridFromSpec(spec)
}
