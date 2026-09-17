package rom

import (
	"fmt"
	"strings"

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
		warps[i] = worldmodel.Warp{X: w.X, Y: w.Y, DestWarpID: w.DestWarpID, DestMap: w.DestMap}
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

// WorldGridSpec lets the existing world.Build APIs consume a Red MapHeader
// through a neutral interface while call sites migrate to profile providers.
func (h MapHeader) WorldGridSpec(romData []byte, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	width := int(h.WidthBlocks) * 2
	height := int(h.HeightBlocks) * 2
	spec := worldmodel.GridSpec{
		MapID:         h.ID,
		Width:         width,
		Height:        height,
		Walkable:      make([]bool, width*height),
		CollisionTile: make([]uint8, width*height),
		FieldTile:     make([]uint8, width*height),
		TilePairs:     redTilePairsForTraversal(romData, h.Tileset, mode),
	}
	for _, ledge := range Ledges(romData, h.Tileset) {
		spec.Ledges = append(spec.Ledges, worldmodel.Ledge{DX: ledge.DX, DY: ledge.DY, From: ledge.From, Over: ledge.Over})
	}
	if width == 0 || height == 0 {
		return spec, nil
	}

	wantBlocks := int(h.WidthBlocks) * int(h.HeightBlocks)
	if blocks == nil {
		var err error
		blocks, err = Blocks(romData, h)
		if err != nil {
			return worldmodel.GridSpec{}, err
		}
	}
	if len(blocks) < wantBlocks {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: block map has %d bytes, want at least %d", h.ID, len(blocks), wantBlocks)
	}
	blocks = blocks[:wantBlocks]

	tsOff, err := bankedOffset(tilesetsBank, tilesetsAddr)
	if err != nil {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: %v", h.ID, err)
	}
	entryOff := tsOff + int(h.Tileset)*tilesetEntryLen
	if entryOff+tilesetEntryLen > len(romData) {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: tileset %d entry at offset %d exceeds ROM of %d bytes", h.ID, h.Tileset, entryOff, len(romData))
	}
	tsBank := romData[entryOff]
	blockPtr := uint16(romData[entryOff+1]) | uint16(romData[entryOff+2])<<8
	collPtr := uint16(romData[entryOff+5]) | uint16(romData[entryOff+6])<<8
	spec.CounterTiles = [3]uint8{romData[entryOff+7], romData[entryOff+8], romData[entryOff+9]}

	collBank := uint8(0)
	if collPtr >= 0x4000 {
		collBank = tsBank
	}
	collOff, err := bankedOffset(collBank, collPtr)
	if err != nil {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: %v", h.ID, err)
	}
	walkableTiles := make([]bool, 256)
	for {
		if collOff >= len(romData) {
			return worldmodel.GridSpec{}, fmt.Errorf("map %d: collision list at offset %d exceeds ROM of %d bytes", h.ID, collOff, len(romData))
		}
		t := romData[collOff]
		collOff++
		if t == 0xff {
			break
		}
		walkableTiles[t] = true
	}

	blockOff, err := bankedOffset(tsBank, blockPtr)
	if err != nil {
		return worldmodel.GridSpec{}, fmt.Errorf("map %d: %v", h.ID, err)
	}
	wb := int(h.WidthBlocks)
	for by := 0; by < int(h.HeightBlocks); by++ {
		for bx := 0; bx < wb; bx++ {
			blockID := blocks[by*wb+bx]
			tilesOff := blockOff + int(blockID)*16
			if tilesOff+16 > len(romData) {
				return worldmodel.GridSpec{}, fmt.Errorf("map %d: block %d data at offset %d exceeds ROM of %d bytes", h.ID, blockID, tilesOff, len(romData))
			}
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					field := romData[tilesOff+(2*sy)*4+2*sx]
					collision := romData[tilesOff+(2*sy+1)*4+2*sx]
					i := (by*2+sy)*width + (bx*2 + sx)
					spec.FieldTile[i] = field
					spec.CollisionTile[i] = collision
					spec.Walkable[i] = walkableTiles[collision]
				}
			}
		}
	}
	return spec, nil
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
