package rom

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/worldmodel"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowTilesetsBank uint8  = 0x03
	yellowTilesetsAddr uint16 = 0x4558
	tilesetEntryLen           = 12
)

type worldProvider struct {
	rom []byte
}

func NewWorldProvider(romData []byte) worldmodel.MapHeaderProvider {
	return &worldProvider{rom: romData}
}

func (p *worldProvider) MapIDs() []uint8 { return MapIDs() }

func (p *worldProvider) ParseMap(mapID uint8) (worldmodel.MapHeader, error) {
	h, err := ParseMap(p.rom, mapID)
	if err != nil {
		return worldmodel.MapHeader{}, err
	}
	warps := make([]worldmodel.Warp, len(h.Warps))
	for i, w := range h.Warps {
		warps[i] = worldmodel.Warp{X: w.X, Y: w.Y, DestWarpID: w.DestWarpID, DestMap: w.DestMap}
	}
	connections := make([]worldmodel.Connection, len(h.Connections))
	for i, c := range h.Connections {
		connections[i] = worldmodel.Connection{Dir: c.Dir, MapID: c.MapID, Offset: c.Offset}
	}
	return worldmodel.MapHeader{
		ID: h.ID, WidthBlocks: h.WidthBlocks, HeightBlocks: h.HeightBlocks,
		Warps: warps, Connections: connections,
	}, nil
}

func (p *worldProvider) Grid(mapID uint8, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	h, err := ParseMap(p.rom, mapID)
	if err != nil {
		return worldmodel.GridSpec{}, err
	}
	return h.WorldGridSpec(p.rom, blocks, mode)
}

func (p *worldProvider) LookupElevator(mapID uint8) (worldmodel.ElevatorSpec, bool) {
	return lookupElevator(mapID)
}

func (p *worldProvider) ElevatorFloorForDestination(elevatorMap, destinationMap uint8) (worldmodel.ElevatorFloor, bool) {
	return elevatorFloorForDestination(elevatorMap, destinationMap)
}

func (h MapHeader) WorldGridSpec(romData []byte, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	return gen1rom.BuildGridSpec(romData, gen1rom.MapHeader(h), blocks, mode, gen1rom.GridLayout{
		TilesetsBank: yellowTilesetsBank, TilesetsAddr: yellowTilesetsAddr, TilesetEntryLen: tilesetEntryLen,
		CollisionBank: yellowCollisionBank,
		TilePairs:     yellowTilePairsForTraversal,
		Ledges:        yellowLedges,
	})
}

// Yellow-owned table addresses (pokeyellow.sym). The byte formats are the
// shared Gen-I ones decoded by gen1rom.
const (
	yellowCollisionBank           uint8 = 0x01                       // Overworld_Coll .. collision lists live in bank 1
	yellowTilePairCollisionsLand        = 0x0ada                     // 00:0ada TilePairCollisionsLand
	yellowTilePairCollisionsWater       = 0x0afc                     // 00:0afc TilePairCollisionsWater
	yellowLedgeTiles                    = 6*0x4000 + 0x6851 - 0x4000 // 06:6851 LedgeTiles
)

func yellowTilePairsForTraversal(romData []byte, tileset uint8, mode worldmodel.TraversalMode) map[[2]uint8]bool {
	addr := yellowTilePairCollisionsLand
	if mode == worldmodel.TraversalWater {
		addr = yellowTilePairCollisionsWater
	}
	return gen1rom.TilePairsAt(romData, addr, tileset)
}

func yellowLedges(romData []byte, tileset uint8) []worldmodel.Ledge {
	return gen1rom.LedgesAt(romData, yellowLedgeTiles, tileset)
}

func isYellowWorldROM(romData []byte) bool {
	return game.InspectROM(romData).SHA1 == sym.ROMSHA1
}

func init() {
	worldmodel.RegisterROMProviderFactory(func(romData []byte) (worldmodel.MapHeaderProvider, bool) {
		if !isYellowWorldROM(romData) {
			return nil, false
		}
		return NewWorldProvider(romData), true
	})
}
