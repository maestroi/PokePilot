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
		TilePairs: yellowTilePairsForTraversal,
		Ledges:    yellowLedges,
	})
}

type yellowPair struct {
	tileset uint8
	a, b    uint8
}

var yellowLandPairs = []yellowPair{
	{17, 0x20, 0x05}, {17, 0x41, 0x05}, {3, 0x30, 0x2E},
	{17, 0x2A, 0x05}, {17, 0x05, 0x21}, {3, 0x52, 0x2E},
	{3, 0x55, 0x2E}, {3, 0x56, 0x2E}, {3, 0x20, 0x2E},
	{3, 0x5E, 0x2E}, {3, 0x5F, 0x2E},
}
var yellowWaterPairs = []yellowPair{
	{3, 0x14, 0x2E}, {3, 0x48, 0x2E}, {17, 0x14, 0x05},
}

func yellowTilePairsForTraversal(_ []byte, tileset uint8, mode worldmodel.TraversalMode) map[[2]uint8]bool {
	src := yellowLandPairs
	if mode == worldmodel.TraversalWater {
		src = yellowWaterPairs
	}
	out := map[[2]uint8]bool{}
	for _, p := range src {
		if p.tileset != tileset {
			continue
		}
		out[[2]uint8{p.a, p.b}] = true
		out[[2]uint8{p.b, p.a}] = true
	}
	return out
}

func yellowLedges(_ []byte, tileset uint8) []worldmodel.Ledge {
	if tileset != 0 {
		return nil
	}
	return []worldmodel.Ledge{
		{DY: 1, From: 0x2C, Over: 0x37},
		{DY: 1, From: 0x39, Over: 0x36},
		{DY: 1, From: 0x39, Over: 0x37},
		{DX: -1, From: 0x2C, Over: 0x27},
		{DX: -1, From: 0x39, Over: 0x27},
		{DX: 1, From: 0x2C, Over: 0x0D},
		{DX: 1, From: 0x2C, Over: 0x1D},
		{DX: 1, From: 0x39, Over: 0x0D},
	}
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
