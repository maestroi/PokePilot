package rom

import (
	"strings"

	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

const (
	redWorldMaxMapID uint8 = 0xF7
	tilesetEntryLen        = 12

	tilePairCollisionsLandAddr  = 0x0c7e
	tilePairCollisionsWaterAddr = 0x0ca0
	tilePairEntryLen            = 3
)

type redWorldProvider struct {
	rom []byte
}

// NewWorldProvider exposes Red/Blue map topology through the portable world
// boundary. The provider owns the ROM bytes so generic graph construction does
// not need to understand a cartridge layout.
func NewWorldProvider(romData []byte) worldmodel.MapHeaderProvider {
	return &redWorldProvider{rom: romData}
}

func (p *redWorldProvider) MapIDs() []uint8 {
	ids := make([]uint8, 0, int(redWorldMaxMapID)+1)
	for id := uint16(0); id <= uint16(redWorldMaxMapID); id++ {
		if validMapID(uint8(id)) {
			ids = append(ids, uint8(id))
		}
	}
	return ids
}

func (p *redWorldProvider) ParseMap(mapID uint8) (worldmodel.MapHeader, error) {
	h, err := ParseMap(p.rom, mapID)
	if err != nil {
		return worldmodel.MapHeader{}, err
	}
	warps := make([]worldmodel.Warp, len(h.Warps))
	for i, w := range h.Warps {
		warps[i] = worldmodel.Warp{
			X: w.X, Y: w.Y, DestWarpID: w.DestWarpID, DestMap: w.DestMap,
			Inert: IsInertWarp(h.ID, w.X, w.Y),
		}
	}
	connections := make([]worldmodel.Connection, len(h.Connections))
	for i, c := range h.Connections {
		connections[i] = worldmodel.Connection{Dir: c.Dir, MapID: c.MapID, Offset: c.Offset}
	}
	return worldmodel.MapHeader{
		ID:           h.ID,
		WidthBlocks:  h.WidthBlocks,
		HeightBlocks: h.HeightBlocks,
		Warps:        warps,
		Connections:  connections,
	}, nil
}

func (p *redWorldProvider) Grid(mapID uint8, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	h, err := ParseMap(p.rom, mapID)
	if err != nil {
		return worldmodel.GridSpec{}, err
	}
	return h.WorldGridSpec(p.rom, blocks, mode)
}

func (p *redWorldProvider) LookupElevator(mapID uint8) (worldmodel.ElevatorSpec, bool) {
	spec, ok := LookupElevator(mapID)
	if !ok {
		return worldmodel.ElevatorSpec{}, false
	}
	floors := make([]worldmodel.ElevatorFloor, len(spec.Floors))
	for i, floor := range spec.Floors {
		floors[i] = worldmodel.ElevatorFloor{MapID: floor.MapID, DestWarpID: floor.DestWarpID}
	}
	return worldmodel.ElevatorSpec{Floors: floors}, true
}

func (p *redWorldProvider) ElevatorFloorForDestination(elevatorMap, destinationMap uint8) (worldmodel.ElevatorFloor, bool) {
	_, floor, _, ok := ElevatorFloorForDestination(elevatorMap, destinationMap)
	if !ok {
		return worldmodel.ElevatorFloor{}, false
	}
	return worldmodel.ElevatorFloor{MapID: floor.MapID, DestWarpID: floor.DestWarpID}, true
}

// WorldGridSpec lets existing Red callers keep passing rom.MapHeader while
// the common Gen-I collision-grid byte decoder lives in gen1rom.
func (h MapHeader) WorldGridSpec(romData []byte, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	return gen1rom.BuildGridSpec(romData, gen1rom.MapHeader(h), blocks, mode, gen1rom.GridLayout{
		TilesetsBank:    tilesetsBank,
		TilesetsAddr:    tilesetsAddr,
		TilesetEntryLen: tilesetEntryLen,
		TilePairs:       redTilePairsForTraversal,
		Ledges: func(data []byte, tileset uint8) []worldmodel.Ledge {
			raw := Ledges(data, tileset)
			out := make([]worldmodel.Ledge, len(raw))
			for i, ledge := range raw {
				out[i] = worldmodel.Ledge{DX: ledge.DX, DY: ledge.DY, From: ledge.From, Over: ledge.Over}
			}
			return out
		},
	})
}

func redTilePairsForTraversal(romData []byte, tileset uint8, mode worldmodel.TraversalMode) map[[2]uint8]bool {
	pairs := map[[2]uint8]bool{}
	addr := tilePairCollisionsLandAddr
	if mode == worldmodel.TraversalWater {
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

func isRegisteredGen1WorldROM(romData []byte) bool {
	if len(romData) < 0x144 {
		return false
	}
	title := strings.TrimRight(string(romData[0x134:0x144]), "\x00 ")
	return title == "POKEMON RED" || title == "POKEMON BLUE"
}

func init() {
	worldmodel.RegisterROMProviderFactory(func(romData []byte) (worldmodel.MapHeaderProvider, bool) {
		if !isRegisteredGen1WorldROM(romData) {
			return nil, false
		}
		return NewWorldProvider(romData), true
	})
}
