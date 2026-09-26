package rom

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

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

func (p *worldProvider) MapIDs() []worldmodel.MapID {
	raw := MapIDs()
	ids := make([]worldmodel.MapID, len(raw))
	for i, id := range raw {
		ids[i] = worldmodel.MapID(id)
	}
	return ids
}

func (p *worldProvider) ParseMap(mapID worldmodel.MapID) (worldmodel.MapHeader, error) {
	if mapID > 0xff {
		return worldmodel.MapHeader{}, fmt.Errorf("yellow world: map id %#04x exceeds Gen-I range", mapID)
	}
	h, err := ParseMap(p.rom, uint8(mapID))
	if err != nil {
		return worldmodel.MapHeader{}, err
	}
	return projectWorldHeader(h), nil
}

func projectWorldHeader(h MapHeader) worldmodel.MapHeader {
	warps := make([]worldmodel.Warp, len(h.Warps))
	for i, w := range h.Warps {
		warps[i] = worldmodel.Warp{X: w.X, Y: w.Y, DestWarpID: w.DestWarpID, DestMap: worldmodel.MapID(w.DestMap)}
	}
	connections := make([]worldmodel.Connection, len(h.Connections))
	for i, c := range h.Connections {
		connections[i] = worldmodel.Connection{Dir: c.Dir, MapID: worldmodel.MapID(c.MapID), Offset: c.Offset}
	}
	objects := make([]worldmodel.MapObject, len(h.Objects))
	for i, object := range h.Objects {
		movement := worldmodel.ObjectMovementUnknown
		switch object.Movement {
		case MovementWalk:
			movement = worldmodel.ObjectMovementWalk
		case MovementStay:
			movement = worldmodel.ObjectMovementStay
		}
		objects[i] = worldmodel.MapObject{
			Slot:               i + 1,
			X:                  object.X,
			Y:                  object.Y,
			Movement:           movement,
			NativeSpriteID:     uint16(object.SpriteID),
			NativeTextID:       uint16(object.TextID),
			NativeItemID:       uint16(object.ItemID),
			NativeTrainerClass: uint16(object.TrainerClass),
			NativeTrainerSet:   uint16(object.TrainerSet),
		}
	}
	return worldmodel.MapHeader{
		ID:            worldmodel.MapID(h.ID),
		NativeTileset: uint16(h.Tileset),
		WidthBlocks:   h.WidthBlocks,
		HeightBlocks:  h.HeightBlocks,
		Warps:         warps,
		Connections:   connections,
		Objects:       objects,
	}
}

func (h MapHeader) WorldMapHeader() worldmodel.MapHeader {
	return projectWorldHeader(h)
}

func (p *worldProvider) Grid(mapID worldmodel.MapID, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	if mapID > 0xff {
		return worldmodel.GridSpec{}, fmt.Errorf("yellow world: map id %#04x exceeds Gen-I range", mapID)
	}
	h, err := ParseMap(p.rom, uint8(mapID))
	if err != nil {
		return worldmodel.GridSpec{}, err
	}
	spec, err := h.WorldGridSpec(p.rom, blocks, mode)
	if err != nil {
		return worldmodel.GridSpec{}, err
	}
	markGen1Cuttable(&spec, h.Tileset)
	if mode == worldmodel.TraversalWater {
		const surfWaterTile uint8 = 0x14
		for i := range spec.Walkable {
			if (i < len(spec.FieldTile) && spec.FieldTile[i] == surfWaterTile) ||
				(i < len(spec.CollisionTile) && spec.CollisionTile[i] == surfWaterTile) {
				spec.Walkable[i] = true
			}
		}
	}
	return spec, nil
}

func (p *worldProvider) LookupElevator(mapID worldmodel.MapID) (worldmodel.ElevatorSpec, bool) {
	if mapID > 0xff {
		return worldmodel.ElevatorSpec{}, false
	}
	spec, ok := lookupElevator(uint8(mapID))
	if !ok {
		return worldmodel.ElevatorSpec{}, false
	}
	for i := range spec.Floors {
		spec.Floors[i].MapID = worldmodel.MapID(spec.Floors[i].MapID)
	}
	return spec, true
}

func (p *worldProvider) ElevatorFloorForDestination(elevatorMap, destinationMap worldmodel.MapID) (worldmodel.ElevatorFloor, bool) {
	if elevatorMap > 0xff || destinationMap > 0xff {
		return worldmodel.ElevatorFloor{}, false
	}
	return elevatorFloorForDestination(uint8(elevatorMap), uint8(destinationMap))
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

func isYellowWorldROM(romData []byte) bool { return IsCartridge(romData) }

// IsCartridge reports whether romData is the supported Yellow image. A
// structural probe (MapHeaderBanks/MapHeaderPointers name PalletTown_h at
// 06:42a1, pokeyellow.sym) rejects other cartridges in three byte reads, so
// hot shared decoders never hash a Red image; the SHA-1 then pins the exact
// revision whose layout this package describes.
func IsCartridge(romData []byte) bool {
	banks, err := Tables.MapHeaderBanks.Offset()
	if err != nil || banks >= len(romData) || romData[banks] != 0x06 {
		return false
	}
	ptrs, err := Tables.MapHeaderPointers.Offset()
	if err != nil || ptrs+1 >= len(romData) || romData[ptrs] != 0xA1 || romData[ptrs+1] != 0x42 {
		return false
	}
	sum := sha1.Sum(romData)
	return hex.EncodeToString(sum[:]) == sym.ROMSHA1
}

func init() {
	worldmodel.RegisterROMProviderFactory(func(romData []byte) (worldmodel.MapHeaderProvider, bool) {
		if !isYellowWorldROM(romData) {
			return nil, false
		}
		return NewWorldProvider(romData), true
	})
}

func markGen1Cuttable(spec *worldmodel.GridSpec, tileset uint8) {
	if spec == nil {
		return
	}
	var tile uint8
	switch tileset {
	case 0: // OVERWORLD
		tile = 0x3d
	case 7: // GYM
		tile = 0x50
	default:
		return
	}
	spec.Cuttable = make([]bool, len(spec.Walkable))
	for i := range spec.Cuttable {
		spec.Cuttable[i] = (i < len(spec.FieldTile) && spec.FieldTile[i] == tile) ||
			(i < len(spec.CollisionTile) && spec.CollisionTile[i] == tile)
	}
}
