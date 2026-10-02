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

// waitLiveMapDimsSettled drives the current map toward a live topology whose
// dimensions match the ROM header, so the block buffer can be decoded as the
// current map's collision. It is the Gen-I half of the same invariant
// waitLiveMapSettled enforces for native routing: a connection or warp makes
// the destination map's identity current before its shell is rebuilt, and the
// block buffer still holds the previous map's bytes through that window.
//
// Gen-I regular warps are the case no other signal catches. They write
// wCurMap to the destination before wCurMapWidth/Height are updated, and they
// leave wJoyIgnore clear through the whole window (measured: ~11 frames,
// run-1mey4xe5t2w04), so Controllable reports true and waitOutScriptedMovement
// does not wait. The fly/dungeon-warp bits are only set by Fly and hole warps,
// not by a door. The dimension match against the ROM header is the positive
// assertion that the new map's shell is installed; it is the only signal a
// regular warp produces.
//
// It returns the settled state, and errLiveMapNotSettled when the budget runs
// out with the dimensions still describing the previous map. A battle or a
// dialogue waiting on the player is not a map shell that is still loading:
// neither hands the overworld back without input, so waiting on them only
// burns the budget and hides the interruption the caller can actually settle.
// They return ErrBattle / ErrDialogueInterrupted at once.
func waitLiveMapDimsSettled(m *emu.Emu, romData []byte) (game.LiveTopologyState, error) {
	routing, err := routingProfileFor(m)
	if err != nil {
		return game.LiveTopologyState{}, err
	}
	var live game.LiveTopologyState
	var header worldmodel.MapHeader
	for waited := 0; waited <= liveMapSettleFrameBudget; waited += liveMapSettlePollFrames {
		overworld := routing.DecodeOverworld(m)
		if overworld.InBattle {
			return game.LiveTopologyState{}, ErrBattle
		}
		if overworld.InDialogue {
			return game.LiveTopologyState{}, ErrDialogueInterrupted
		}
		decoded, err := routing.DecodeLiveTopology(m)
		if err != nil {
			return game.LiveTopologyState{}, err
		}
		h, err := routingHeaderForROM(romData, uint8(decoded.NativeMapID))
		if err != nil {
			return game.LiveTopologyState{}, err
		}
		header = h
		if decoded.WidthBlocks == int(h.WidthBlocks) && decoded.HeightBlocks == int(h.HeightBlocks) {
			return decoded, nil
		}
		live = decoded
		m.StepFrames(liveMapSettlePollFrames)
	}
	return live, fmt.Errorf("%w: map %#04x is %dx%d blocks, ROM header says %dx%d after %d frames",
		errLiveMapNotSettled, live.NativeMapID, live.WidthBlocks, live.HeightBlocks,
		header.WidthBlocks, header.HeightBlocks, liveMapSettleFrameBudget)
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
