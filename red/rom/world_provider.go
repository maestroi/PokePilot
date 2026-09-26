package rom

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

const (
	redWorldMaxMapID uint8 = 0xF7
	tilesetEntryLen        = 12
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

func (p *redWorldProvider) MapIDs() []worldmodel.MapID {
	ids := make([]worldmodel.MapID, 0, int(redWorldMaxMapID)+1)
	for id := uint16(0); id <= uint16(redWorldMaxMapID); id++ {
		if validMapID(p.rom, uint8(id)) {
			ids = append(ids, worldmodel.MapID(id))
		}
	}
	return ids
}

func (p *redWorldProvider) ParseMap(mapID worldmodel.MapID) (worldmodel.MapHeader, error) {
	if mapID > 0xff {
		return worldmodel.MapHeader{}, fmt.Errorf("red world: map id %#04x exceeds Gen-I range", mapID)
	}
	h, err := ParseMap(p.rom, uint8(mapID))
	if err != nil {
		return worldmodel.MapHeader{}, err
	}
	out := projectWorldHeader(h)
	actors, err := SpecialInteractionActors(p.rom, mapID)
	if err != nil {
		return worldmodel.MapHeader{}, err
	}
	roles := make(map[[2]uint8]worldmodel.InteractionRole, len(actors))
	for _, actor := range actors {
		roles[[2]uint8{actor.X, actor.Y}] = actor.Role
	}
	for i := range out.Objects {
		out.Objects[i].Role = roles[[2]uint8{out.Objects[i].X, out.Objects[i].Y}]
	}
	return out, nil
}

func projectWorldHeader(h MapHeader) worldmodel.MapHeader {
	warps := make([]worldmodel.Warp, len(h.Warps))
	for i, w := range h.Warps {
		warps[i] = worldmodel.Warp{
			X: w.X, Y: w.Y, DestWarpID: w.DestWarpID, DestMap: worldmodel.MapID(w.DestMap),
			Inert: IsInertWarp(h.ID, w.X, w.Y),
		}
	}
	connections := make([]worldmodel.Connection, len(h.Connections))
	for i, c := range h.Connections {
		connections[i] = worldmodel.Connection{Dir: c.Dir, MapID: worldmodel.MapID(c.MapID), Offset: c.Offset}
	}
	objects := make([]worldmodel.MapObject, len(h.Objects))
	for i, object := range h.Objects {
		objects[i] = worldmodel.MapObject{
			Slot:               i + 1,
			X:                  object.X,
			Y:                  object.Y,
			Movement:           worldObjectMovement(object.Movement),
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

func worldObjectMovement(movement uint8) worldmodel.ObjectMovement {
	switch movement {
	case MovementWalk:
		return worldmodel.ObjectMovementWalk
	case MovementStay:
		return worldmodel.ObjectMovementStay
	default:
		return worldmodel.ObjectMovementUnknown
	}
}

// WorldMapHeader lets legacy Red-owned callers pass their richer header through
// the generic routing boundary without exposing Red's concrete type.
func (h MapHeader) WorldMapHeader() worldmodel.MapHeader {
	return projectWorldHeader(h)
}

func (p *redWorldProvider) Grid(mapID worldmodel.MapID, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	if mapID > 0xff {
		return worldmodel.GridSpec{}, fmt.Errorf("red world: map id %#04x exceeds Gen-I range", mapID)
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
		enableGen1SurfWater(&spec)
	}
	return spec, nil
}

func enableGen1SurfWater(spec *worldmodel.GridSpec) {
	if spec == nil {
		return
	}
	const surfWaterTile uint8 = 0x14
	for i := range spec.Walkable {
		if (i < len(spec.FieldTile) && spec.FieldTile[i] == surfWaterTile) ||
			(i < len(spec.CollisionTile) && spec.CollisionTile[i] == surfWaterTile) {
			spec.Walkable[i] = true
		}
	}
}

func (p *redWorldProvider) LookupElevator(mapID worldmodel.MapID) (worldmodel.ElevatorSpec, bool) {
	if mapID > 0xff {
		return worldmodel.ElevatorSpec{}, false
	}
	spec, ok := LookupElevator(uint8(mapID))
	if !ok {
		return worldmodel.ElevatorSpec{}, false
	}
	floors := make([]worldmodel.ElevatorFloor, len(spec.Floors))
	for i, floor := range spec.Floors {
		floors[i] = worldmodel.ElevatorFloor{MapID: worldmodel.MapID(floor.MapID), DestWarpID: floor.DestWarpID}
	}
	return worldmodel.ElevatorSpec{PanelX: spec.PanelX, PanelY: spec.PanelY, Floors: floors}, true
}

func (p *redWorldProvider) ElevatorFloorForDestination(elevatorMap, destinationMap worldmodel.MapID) (worldmodel.ElevatorFloor, bool) {
	if elevatorMap > 0xff || destinationMap > 0xff {
		return worldmodel.ElevatorFloor{}, false
	}
	_, floor, _, ok := ElevatorFloorForDestination(uint8(elevatorMap), uint8(destinationMap))
	if !ok {
		return worldmodel.ElevatorFloor{}, false
	}
	return worldmodel.ElevatorFloor{MapID: floor.MapID, DestWarpID: floor.DestWarpID}, true
}

// WorldGridSpec lets existing Red callers keep passing rom.MapHeader while
// the common Gen-I collision-grid byte decoder lives in gen1rom.
func (h MapHeader) WorldGridSpec(romData []byte, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	layout := Tables(romData)
	return gen1rom.BuildGridSpec(romData, gen1rom.MapHeader(h), blocks, mode, gen1rom.GridLayout{
		TilesetsBank:    layout.Tilesets.Bank,
		TilesetsAddr:    layout.Tilesets.Addr,
		TilesetEntryLen: tilesetEntryLen,
		CollisionBank:   layout.CollisionBank,
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
	layout := Tables(romData)
	table := layout.TilePairCollisionsLand
	if mode == worldmodel.TraversalWater {
		table = layout.TilePairCollisionsWater
	}
	off, err := table.Offset()
	if err != nil {
		return map[[2]uint8]bool{}
	}
	return gen1rom.TilePairsAt(romData, off, tileset)
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
