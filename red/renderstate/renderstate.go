// Package renderstate translates Pokémon Red's ROM/RAM state into the
// game-agnostic semantic renderstate protocol.
package renderstate

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	protocol "github.com/maestroi/pokepilot/renderstate"
	"github.com/maestroi/pokepilot/world"
)

const (
	redOverworldTileset uint8 = 0x00
	redWaterTile        uint8 = 0x14
	redCutTreeTile      uint8 = 0x3d
	redGrassTile        uint8 = 0x52
	liveMapBorderBlocks       = 3

	tileSign protocol.TileKind = "sign"

	movementBike protocol.MovementKind = "bike"
	movementSurf protocol.MovementKind = "surf"
)

// Producer owns the Pokémon Red-specific extraction needed to enrich the
// portable RenderState contract. It accepts only the exact supported Red ROM
// revision so static map semantics cannot silently drift across ROM versions.
type Producer struct {
	rom     []byte
	profile *redprofile.Profile
}

// New validates romData against the supported Pokémon Red profile and returns
// a semantic render-state producer for that exact revision.
func New(romData []byte) (*Producer, error) {
	profile := redprofile.New()
	info := game.InspectROM(romData)
	if !profile.Detect(info) {
		return nil, fmt.Errorf("red renderstate: unsupported ROM title=%q sha1=%s", info.Title, info.SHA1)
	}
	return &Producer{rom: append([]byte(nil), romData...), profile: profile}, nil
}

// StaticMap reconstructs one Red map semantically from ROM data without using
// the Game Boy framebuffer. Script-mutated live geometry is handled by Snapshot.
func (p *Producer) StaticMap(mapID uint8) (*protocol.MapState, []protocol.TileLayer, error) {
	header, err := rom.ParseMap(p.rom, mapID)
	if err != nil {
		return nil, nil, fmt.Errorf("red renderstate: parse map %02x: %w", mapID, err)
	}
	grid, err := world.Build(p.rom, header)
	if err != nil {
		return nil, nil, fmt.Errorf("red renderstate: build map %02x: %w", mapID, err)
	}
	mapState, err := mapStateFor(header, grid)
	if err != nil {
		return nil, nil, err
	}
	return mapState, semanticLayers(p.rom, header, grid), nil
}

// Snapshot captures one coherent RAM observation and enriches the generic
// profile baseline with Red-owned live map geometry, visible entities, and
// player movement. If the current map buffer is not safely decodable, callers
// should fall back to the classic framebuffer rather than render stale geometry.
func (p *Producer) Snapshot(reader game.MemoryReader, meta protocol.FrameMeta) (protocol.RenderState, error) {
	if reader == nil {
		return protocol.RenderState{}, fmt.Errorf("red renderstate: memory reader is required")
	}

	var mem state.Mem
	reader.PeekInto(0, mem[:])
	obs, err := p.profile.DecodeObservation(memorySnapshot{mem: &mem}, nil)
	if err != nil {
		return protocol.RenderState{}, fmt.Errorf("red renderstate: decode observation: %w", err)
	}
	meta.GameID = p.profile.ID()
	meta.Revision = p.profile.Revision()
	out := protocol.FromProfileObservation(meta, obs)

	// Battle presentation is authoritative on its own and must not depend on
	// overworld geometry remaining decodable while the battle engine owns the
	// screen. This also prevents transient map-buffer churn from knocking a
	// Modern viewer back to the framebuffer during battles.
	if battle := semanticBattle(p.rom, &mem); battle != nil {
		out.Scene = protocol.SceneBattle
		out.Battle = battle
		out.Capabilities = addCapabilities(out.Capabilities, protocol.CapabilityBattle)
		if menu := semanticMenu(&mem); menu != nil {
			out.Menu = menu
			out.Capabilities = addCapabilities(out.Capabilities, protocol.CapabilityMenu)
		}
		if err := protocol.Validate(out); err != nil {
			return protocol.RenderState{}, err
		}
		return out, nil
	}

	if menu := semanticMenu(&mem); menu != nil {
		out.Scene = protocol.SceneMenu
		out.Menu = menu
		out.Capabilities = addCapabilities(out.Capabilities, protocol.CapabilityMenu)
	} else if dialogue := semanticDialogue(&mem); dialogue != nil {
		out.Scene = protocol.SceneDialogue
		out.Dialogue = dialogue
		out.Capabilities = addCapabilities(out.Capabilities, protocol.CapabilityDialogue)
	} else {
		out.Scene = protocol.SceneOverworld
	}

	header, err := rom.ParseMap(p.rom, uint8(obs.NativeMapID))
	if err != nil {
		if out.Scene != protocol.SceneOverworld {
			return validatePresentationOnly(out)
		}
		return protocol.RenderState{}, fmt.Errorf("red renderstate: parse live map %02x: %w", obs.NativeMapID, err)
	}
	blocks, err := liveMapBlocks(&mem, header)
	if err != nil {
		if out.Scene != protocol.SceneOverworld {
			return validatePresentationOnly(out)
		}
		return protocol.RenderState{}, err
	}
	grid, err := world.BuildFromBlocks(p.rom, header, blocks)
	if err != nil {
		if out.Scene != protocol.SceneOverworld {
			return validatePresentationOnly(out)
		}
		return protocol.RenderState{}, fmt.Errorf("red renderstate: build live map %02x: %w", header.ID, err)
	}
	mapState, err := mapStateFor(header, grid)
	if err != nil {
		if out.Scene != protocol.SceneOverworld {
			return validatePresentationOnly(out)
		}
		return protocol.RenderState{}, err
	}
	out.Map = mapState
	out.Layers = semanticLayers(p.rom, header, grid)
	out.Entities = liveEntities(header, &mem, out.Map.ID)
	out.Capabilities = addCapabilities(out.Capabilities, protocol.CapabilityLayers, protocol.CapabilityEntities)
	if out.Player != nil {
		out.Player.Movement = playerMovement(&mem, out.Player.Position)
	}
	if err := protocol.Validate(out); err != nil {
		return protocol.RenderState{}, err
	}
	return out, nil
}

func validatePresentationOnly(out protocol.RenderState) (protocol.RenderState, error) {
	if err := protocol.Validate(out); err != nil {
		return protocol.RenderState{}, err
	}
	return out, nil
}

type memorySnapshot struct {
	mem *state.Mem
}

func (m memorySnapshot) Peek8(addr uint16) byte { return m.mem.U8(addr) }
func (m memorySnapshot) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):int(addr)+len(dst)])
}

func semanticMapID(mapName string) game.PlaceID {
	return game.CanonicalID(strings.ReplaceAll(mapName, "_", " "))
}

func mapStateFor(header rom.MapHeader, grid *world.Grid) (*protocol.MapState, error) {
	name := state.MapName(header.ID)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("red renderstate: map %02x has no semantic name", header.ID)
	}
	return &protocol.MapState{
		ID:     semanticMapID(name),
		Name:   name,
		Width:  grid.Width,
		Height: grid.Height,
	}, nil
}

func semanticLayers(romData []byte, header rom.MapHeader, grid *world.Grid) []protocol.TileLayer {
	terrain := protocol.TileLayer{
		ID:     "terrain",
		Kind:   protocol.LayerTerrain,
		Width:  grid.Width,
		Height: grid.Height,
		Cells:  make([]protocol.TileCell, grid.Width*grid.Height),
	}
	objects := protocol.TileLayer{
		ID:     "objects",
		Kind:   protocol.LayerObjects,
		Width:  grid.Width,
		Height: grid.Height,
		Cells:  make([]protocol.TileCell, grid.Width*grid.Height),
	}
	ledgeOverTiles := make(map[uint8]struct{})
	for _, ledge := range rom.Ledges(romData, header.Tileset) {
		ledgeOverTiles[ledge.Over] = struct{}{}
	}
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			i := y*grid.Width + x
			terrain.Cells[i] = semanticTerrainCell(header.Tileset, grid, x, y, ledgeOverTiles)
			objects.Cells[i] = protocol.TileCell{Kind: protocol.TileUnknown}
		}
	}
	for _, sign := range header.Signs {
		setLayerCell(&objects, int(sign.X), int(sign.Y), protocol.TileCell{Kind: tileSign})
	}
	for _, warp := range header.Warps {
		setLayerCell(&objects, int(warp.X), int(warp.Y), protocol.TileCell{Kind: protocol.TileWarp})
	}
	return []protocol.TileLayer{terrain, objects}
}

func semanticTerrainCell(tileset uint8, grid *world.Grid, x, y int, ledgeOverTiles map[uint8]struct{}) protocol.TileCell {
	field, fieldOK := grid.FieldTile(x, y)
	collision, collisionOK := grid.Tile(x, y)
	_, ledge := ledgeOverTiles[collision]
	return protocol.TileCell{Kind: semanticTerrainKind(tileset, grid.Walkable(x, y), field, fieldOK, collision, collisionOK, ledge)}
}

func semanticTerrainKind(tileset uint8, walkable bool, field uint8, fieldOK bool, collision uint8, collisionOK bool, ledge bool) protocol.TileKind {
	matches := func(tile uint8) bool {
		return fieldOK && field == tile || collisionOK && collision == tile
	}

	switch {
	case matches(redWaterTile):
		return protocol.TileWater
	case tileset == redOverworldTileset && matches(redCutTreeTile):
		return protocol.TileTree
	case tileset == redOverworldTileset && matches(redGrassTile):
		return protocol.TileGrass
	case ledge:
		return protocol.TileLedge
	case walkable:
		return protocol.TilePath
	default:
		return protocol.TileWall
	}
}

func setLayerCell(layer *protocol.TileLayer, x, y int, cell protocol.TileCell) {
	if x < 0 || y < 0 || x >= layer.Width || y >= layer.Height {
		return
	}
	layer.Cells[y*layer.Width+x] = cell
}

func liveMapBlocks(mem *state.Mem, header rom.MapHeader) ([]byte, error) {
	if mem.U8(sym.CurMap) != header.ID {
		return nil, fmt.Errorf("red renderstate: RAM map %02x does not match header %02x", mem.U8(sym.CurMap), header.ID)
	}
	widthBlocks, heightBlocks := int(header.WidthBlocks), int(header.HeightBlocks)
	if got := int(mem.U8(sym.CurMapWidth)); got != widthBlocks {
		return nil, fmt.Errorf("red renderstate: live map width is %d blocks, ROM header says %d", got, widthBlocks)
	}
	if got := int(mem.U8(sym.CurMapHeight)); got != heightBlocks {
		return nil, fmt.Errorf("red renderstate: live map height is %d blocks, ROM header says %d", got, heightBlocks)
	}
	if widthBlocks == 0 || heightBlocks == 0 {
		return []byte{}, nil
	}
	stride := widthBlocks + 2*liveMapBorderBlocks
	first := liveMapBorderBlocks*stride + liveMapBorderBlocks
	last := first + (heightBlocks-1)*stride + widthBlocks - 1
	if last >= sym.OverworldMapLen {
		return nil, fmt.Errorf("red renderstate: live map %dx%d exceeds block buffer", widthBlocks, heightBlocks)
	}
	blocks := make([]byte, widthBlocks*heightBlocks)
	for y := 0; y < heightBlocks; y++ {
		for x := 0; x < widthBlocks; x++ {
			off := first + y*stride + x
			blocks[y*widthBlocks+x] = mem.U8(sym.OverworldMap + uint16(off))
		}
	}
	return blocks, nil
}

func liveEntities(header rom.MapHeader, mem *state.Mem, mapID game.PlaceID) []protocol.ActorState {
	sprites := state.DecodeSprites(mem)
	out := make([]protocol.ActorState, 0, len(sprites))
	for _, sprite := range sprites {
		if sprite.Slot <= 0 || sprite.Slot > len(header.Objects) {
			continue
		}
		object := header.Objects[sprite.Slot-1]
		kind := objectKind(object)
		appearance := spriteAppearance(sprite.PictureID)
		actor := protocol.ActorState{
			ID:         fmt.Sprintf("%s:object:%d", mapID, sprite.Slot),
			Kind:       kind,
			Appearance: appearance,
			Position:   protocol.Position{X: sprite.X, Y: sprite.Y},
		}
		if appearance != "unknown" {
			actor.Label = strings.ReplaceAll(appearance, "_", " ")
		}
		if kind == protocol.EntityNPC || kind == protocol.EntityTrainer {
			actor.Facing = directionForFacing(mem.U8(sym.SpritePlayerStateData1 + uint16(sprite.Slot)*0x10 + 0x09))
		}
		out = append(out, actor)
	}
	return out
}

func objectKind(object rom.Object) protocol.EntityKind {
	switch {
	case object.TextID&0x40 != 0:
		return protocol.EntityTrainer
	case object.TextID&0x80 != 0:
		return protocol.EntityItem
	case object.SpriteID >= 0x3d:
		return protocol.EntityObject
	default:
		return protocol.EntityNPC
	}
}

func directionForFacing(raw byte) protocol.Direction {
	switch state.Facing(raw) {
	case state.FacingDown:
		return protocol.DirectionDown
	case state.FacingUp:
		return protocol.DirectionUp
	case state.FacingLeft:
		return protocol.DirectionLeft
	case state.FacingRight:
		return protocol.DirectionRight
	default:
		return protocol.DirectionUnknown
	}
}

func playerMovement(mem *state.Mem, at protocol.Position) *protocol.MovementState {
	if mem.U8(sym.WalkCounter) == 0 {
		return &protocol.MovementState{Kind: protocol.MovementIdle, From: at, To: at}
	}

	dx, dy := 0, 0
	switch mem.U8(sym.PlayerMovingDirection) {
	case 0x01:
		dx = 1
	case 0x02:
		dx = -1
	case 0x04:
		dy = 1
	case 0x08:
		dy = -1
	default:
		return &protocol.MovementState{Kind: protocol.MovementUnknown, From: at, To: at}
	}
	kind := protocol.MovementWalk
	switch mem.U8(sym.WalkBikeSurfState) {
	case 1:
		kind = movementBike
	case 2:
		kind = movementSurf
	}
	return &protocol.MovementState{
		Kind: kind,
		From: at,
		To:   protocol.Position{X: at.X + dx, Y: at.Y + dy},
	}
}

func addCapabilities(existing []protocol.Capability, capabilities ...protocol.Capability) []protocol.Capability {
	out := append([]protocol.Capability(nil), existing...)
	for _, capability := range capabilities {
		found := false
		for _, current := range out {
			if current == capability {
				found = true
				break
			}
		}
		if !found {
			out = append(out, capability)
		}
	}
	return out
}

func spriteAppearance(id uint8) string {
	if int(id) < len(redSpriteAppearances) {
		if name := redSpriteAppearances[id]; name != "" {
			return name
		}
	}
	return "unknown"
}

var redSpriteAppearances = []string{
	"unknown",
	"player",
	"rival",
	"professor_oak",
	"youngster",
	"monster",
	"cooltrainer_f",
	"cooltrainer_m",
	"little_girl",
	"bird",
	"middle_aged_man",
	"gambler",
	"super_nerd",
	"girl",
	"hiker",
	"beauty",
	"gentleman",
	"daisy",
	"biker",
	"sailor",
	"cook",
	"bike_shop_clerk",
	"mr_fuji",
	"giovanni",
	"rocket",
	"channeler",
	"waiter",
	"silph_worker_f",
	"middle_aged_woman",
	"brunette_girl",
	"lance",
	"scientist",
	"scientist",
	"rocker",
	"swimmer",
	"safari_zone_worker",
	"gym_guide",
	"gramps",
	"clerk",
	"fishing_guru",
	"granny",
	"nurse",
	"link_receptionist",
	"silph_president",
	"silph_worker_m",
	"warden",
	"captain",
	"fisher",
	"koga",
	"guard",
	"guard",
	"mom",
	"balding_guy",
	"little_boy",
	"gameboy_kid",
	"gameboy_kid",
	"fairy",
	"agatha",
	"bruno",
	"lorelei",
	"seel",
	"poke_ball",
	"fossil",
	"boulder",
	"paper",
	"pokedex",
	"clipboard",
	"snorlax",
	"old_amber",
	"old_amber",
	"sleeping_gambler",
	"sleeping_gambler",
	"sleeping_gambler",
}
