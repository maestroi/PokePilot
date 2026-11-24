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

// gen2MapShellPhase maps wMapStatus onto the cartridge's own map entry
// sequence (constants/ram_constants.asm: MAPSTATUS_START..MAPSTATUS_DONE).
// The block buffer is only rebuilt during MAPSTATUS_ENTER, so a reader that
// observes the new map identity before the phase reaches MAPSTATUS_HANDLE is
// looking at a half-written buffer rather than at map data.
func gen2MapShellPhase(reader game.MemoryReader) game.MapShellPhase {
	switch reader.Peek8(sym.MapStatus) {
	case gen2MapStatusStart:
		return game.MapShellStarting
	case gen2MapStatusEnter:
		return game.MapShellEntering
	case gen2MapStatusHandle:
		return game.MapShellSettled
	case gen2MapStatusDone:
		return game.MapShellDone
	default:
		return game.MapShellUnknown
	}
}

// gen2SpriteRock is SPRITE_ROCK in pokegold/constants/sprite_constants.asm.
const (
	gen2SpriteRock               byte = 0x59
	gen2SpriteBoulder            byte = 0x5a
	gen2StrengthBoulderMovement  byte = 0x19
	gen2ObjectMovementTypeOffset      = 0x03
)

// gen2StationaryMovement lists SPRITEMOVEDATA_* types that never leave their
// tile (pokegold constants/map_object_constants.asm): STILL, STANDING_*, and
// the in-place spinners.
var gen2StationaryMovement = map[byte]bool{
	0x01: true, 0x06: true, 0x07: true, 0x08: true, 0x09: true,
	0x03: true, 0x0a: true, 0x1e: true, 0x1f: true,
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
		var clearable game.FieldMoveID
		// Every SPRITE_ROCK object in pokegold is SPRITEMOVEDATA_SMASHABLE_ROCK
		// (verified against data/maps/objects), so the live sprite id is enough
		// without reading wMapObjects.
		if reader.Peek8(base) == gen2SpriteRock {
			clearable = game.FieldMoveRockSmash
		}
		movement := reader.Peek8(base + gen2ObjectMovementTypeOffset)
		objects = append(objects, game.LiveMapObject{
			Slot: slot, X: x, Y: y, Clearable: clearable,
			Pushable:   reader.Peek8(base) == gen2SpriteBoulder && movement == gen2StrengthBoulderMovement,
			Stationary: gen2StationaryMovement[movement],
		})
		positions[slot] = game.MapPoint{X: x, Y: y}
	}
	// Object structs exist only for objects near the screen. A standing NPC
	// off screen still blocks its tile; without it routing oscillates
	// between two corridors whose blockers are never visible together
	// (Route 13's Pokéfan Joshua and Bird Keeper Perry). wMapObjects keeps
	// every object's tile; wObjectMasks hides the ones that are not there.
	for i := 1; i < sym.NumMapObjects; i++ {
		base := sym.MapObjects + uint16(i*sym.MapObjectLen)
		if _, loaded := positions[i]; loaded || reader.Peek8(sym.ObjectMasks+uint16(i)) != 0 ||
			reader.Peek8(base+1) == 0 || !gen2StationaryMovement[reader.Peek8(base+4)] {
			continue
		}
		rawY, rawX := reader.Peek8(base+2), reader.Peek8(base+3)
		if rawX < 4 || rawY < 4 {
			continue
		}
		objects = append(objects, game.LiveMapObject{Slot: i, X: int(rawX) - 4, Y: int(rawY) - 4, Stationary: true})
	}
	return objects, positions
}

// DecodeLiveTopology exposes the mutable half of Gen-II routing: current map
// blocks, traversal mode and active object positions. Static warps/connections
// remain owned by the native world provider.
func (p *Profile) DecodeLiveTopology(reader game.MemoryReader) (game.LiveTopologyState, error) {
	if reader == nil {
		return game.LiveTopologyState{}, fmt.Errorf("gs profile: nil routing memory reader")
	}
	width, height := int(reader.Peek8(sym.MapWidth)), int(reader.Peek8(sym.MapHeight))
	blocks, err := decodeGen2LiveMapBlocks(reader, width, height)
	if err != nil {
		return game.LiveTopologyState{}, err
	}
	phase := gen2MapShellPhase(reader)
	// The block buffer is decodable exactly when the overworld has handed
	// control back: MapStatus reaches HANDLE *before* the shell is rebuilt, so
	// it is necessary but not sufficient. A field script that still owns the
	// overworld leaves the previous map's bytes in the buffer while the map
	// identity already names the new map.
	controllable := gsControllable(reader)
	objects, positions := decodeGen2LiveObjects(reader)
	return game.LiveTopologyState{
		NativeMapID:     gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber)),
		WidthBlocks:     width,
		HeightBlocks:    height,
		Blocks:          blocks,
		Traversal:       gen2Traversal(reader),
		MapShellPhase:   phase,
		BlocksSettled:   controllable && width > 0 && height > 0,
		LiveObjects:     objects,
		ObjectPositions: positions,
		HiddenObjects:   map[int]bool{},
		ScriptBarriers:  p.gen2ScriptBarriers(reader),
	}, nil
}

// gen2BicycleItem is BICYCLE in pokegold constants/item_constants.asm.
const gen2BicycleItem = 0x07

// gen2BicycleGates are the Cycling Road gates whose coord events
// (Route16GateBicycleCheck, Route17Route18GateBicycleCheck) run
// `checkitem BICYCLE` and push the player back without one.
var gen2BicycleGates = map[string][]game.MapPoint{
	"ROUTE_16_GATE":          {{X: 5, Y: 4}, {X: 5, Y: 5}},
	"ROUTE_17_ROUTE_18_GATE": {{X: 5, Y: 4}, {X: 5, Y: 5}},
}

func (p *Profile) gen2ScriptBarriers(reader game.MemoryReader) []game.MapPoint {
	info, ok := gsdata.Map(gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber)))
	if !ok {
		return nil
	}
	gate := gen2BicycleGates[info.Name]
	if gate == nil {
		return nil
	}
	for _, item := range p.DecodeInventory(reader).Items {
		if item.NativeItemID == gen2BicycleItem {
			return nil
		}
	}
	return gate
}
