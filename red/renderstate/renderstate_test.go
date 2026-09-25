package renderstate

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	protocol "github.com/maestroi/pokepilot/renderstate"
)

func TestSemanticTerrainKindUsesRedOwnedTileMeaning(t *testing.T) {
	tests := []struct {
		name      string
		tileset   uint8
		walkable  bool
		field     uint8
		fieldOK   bool
		collision uint8
		collOK    bool
		ledge     bool
		want      protocol.TileKind
	}{
		{name: "water", walkable: false, collision: redWaterTile, collOK: true, want: protocol.TileWater},
		{name: "overworld tree", tileset: redOverworldTileset, field: redCutTreeTile, fieldOK: true, want: protocol.TileTree},
		{name: "overworld grass", tileset: redOverworldTileset, field: redGrassTile, fieldOK: true, walkable: true, want: protocol.TileGrass},
		{name: "same byte is not a tree in another tileset", tileset: 3, field: redCutTreeTile, fieldOK: true, want: protocol.TileWall},
		{name: "ledge from ROM ledge table", tileset: redOverworldTileset, collision: 0x2c, collOK: true, ledge: true, want: protocol.TileLedge},
		{name: "unknown walkable tile", tileset: 7, field: 0xee, fieldOK: true, walkable: true, want: protocol.TilePath},
		{name: "unknown blocked tile", tileset: 7, collision: 0xee, collOK: true, want: protocol.TileWall},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := semanticTerrainKind(tt.tileset, tt.walkable, tt.field, tt.fieldOK, tt.collision, tt.collOK, tt.ledge); got != tt.want {
				t.Fatalf("semanticTerrainKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestObjectKindAndAppearanceHaveSemanticFallbacks(t *testing.T) {
	if got := objectKind(rom.Object{TextID: 0x40}); got != protocol.EntityTrainer {
		t.Fatalf("trainer kind = %q", got)
	}
	if got := objectKind(rom.Object{TextID: 0x80}); got != protocol.EntityItem {
		t.Fatalf("item kind = %q", got)
	}
	if got := objectKind(rom.Object{SpriteID: 0x3f}); got != protocol.EntityObject {
		t.Fatalf("object kind = %q", got)
	}
	if got := objectKind(rom.Object{SpriteID: 0x04}); got != protocol.EntityNPC {
		t.Fatalf("npc kind = %q", got)
	}
	if got := spriteAppearance(0x03); got != "professor_oak" {
		t.Fatalf("Oak appearance = %q", got)
	}
	if got := spriteAppearance(0xff); got != "unknown" {
		t.Fatalf("unknown appearance = %q", got)
	}
}

func TestLiveEntitiesUseRAMPositionFacingAndStableMapObjectIdentity(t *testing.T) {
	header := rom.MapHeader{
		Objects: []rom.Object{
			{SpriteID: 0x04, TextID: 0x01},
			{SpriteID: 0x0e, TextID: 0x42},
			{SpriteID: 0x3d, TextID: 0x83},
		},
	}
	var mem state.Mem
	writeLiveSprite(&mem, 1, 9, 11, 0x04, byte(state.FacingLeft))
	writeLiveSprite(&mem, 2, 13, 7, 0x0e, byte(state.FacingUp))
	writeLiveSprite(&mem, 3, 5, 4, 0x3d, byte(state.FacingDown))

	got := liveEntities(header, &mem, "pallet town")
	if len(got) != 3 {
		t.Fatalf("entities = %d, want 3: %+v", len(got), got)
	}
	if got[0].ID != "pallet town:object:1" || got[0].Kind != protocol.EntityNPC || got[0].Position != (protocol.Position{X: 9, Y: 11}) || got[0].Facing != protocol.DirectionLeft {
		t.Fatalf("npc = %+v", got[0])
	}
	if got[1].ID != "pallet town:object:2" || got[1].Kind != protocol.EntityTrainer || got[1].Position != (protocol.Position{X: 13, Y: 7}) || got[1].Facing != protocol.DirectionUp {
		t.Fatalf("trainer = %+v", got[1])
	}
	if got[2].Kind != protocol.EntityItem || got[2].Appearance != "poke_ball" || got[2].Facing != "" {
		t.Fatalf("item = %+v", got[2])
	}
}

func TestPlayerMovementUsesAuthoritativeDirectionAndMode(t *testing.T) {
	at := protocol.Position{X: 7, Y: 8}
	var mem state.Mem
	if got := playerMovement(&mem, at); got.Kind != protocol.MovementIdle || got.From != at || got.To != at {
		t.Fatalf("idle movement = %+v", got)
	}

	mem[sym.WalkCounter] = 4
	mem[sym.PlayerMovingDirection] = 0x01
	if got := playerMovement(&mem, at); got.Kind != protocol.MovementWalk || got.From != at || got.To != (protocol.Position{X: 8, Y: 8}) {
		t.Fatalf("walk movement = %+v", got)
	}

	mem[sym.PlayerMovingDirection] = 0x08
	mem[sym.WalkBikeSurfState] = 2
	if got := playerMovement(&mem, at); got.Kind != movementSurf || got.To != (protocol.Position{X: 7, Y: 7}) {
		t.Fatalf("surf movement = %+v", got)
	}

	mem[sym.PlayerMovingDirection] = 0xff
	if got := playerMovement(&mem, at); got.Kind != protocol.MovementUnknown || got.To != at {
		t.Fatalf("unknown movement = %+v", got)
	}
}

func TestLiveMapBlocksReadsBorderedCurrentMapBuffer(t *testing.T) {
	header := rom.MapHeader{ID: 3, WidthBlocks: 2, HeightBlocks: 2}
	var mem state.Mem
	mem[sym.CurMap] = header.ID
	mem[sym.CurMapWidth] = header.WidthBlocks
	mem[sym.CurMapHeight] = header.HeightBlocks
	writeLiveBlocks(&mem, header, []byte{0x11, 0x12, 0x21, 0x22})

	got, err := liveMapBlocks(&mem, header)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x11, 0x12, 0x21, 0x22}
	if len(got) != len(want) {
		t.Fatalf("blocks = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("blocks[%d] = %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestNewRejectsUnknownROM(t *testing.T) {
	if _, err := New(make([]byte, 0x150)); err == nil {
		t.Fatal("New(unknown ROM) = nil error")
	}
}

func TestStaticMapReconstructsPalletAndViridianSemantically(t *testing.T) {
	producer := loadRedProducer(t)
	tests := []struct {
		mapID      uint8
		wantID     string
		wantWidth  int
		wantHeight int
		wantWarps  int
	}{
		{mapID: 0x00, wantID: "pallet town", wantWidth: 20, wantHeight: 18, wantWarps: 3},
		{mapID: 0x0c, wantID: "route 1", wantWidth: 20, wantHeight: 36, wantWarps: 0},
		{mapID: 0x01, wantID: "viridian city", wantWidth: 40, wantHeight: 36, wantWarps: 5},
	}
	for _, tt := range tests {
		t.Run(tt.wantID, func(t *testing.T) {
			m, layers, err := producer.StaticMap(tt.mapID)
			if err != nil {
				t.Fatal(err)
			}
			if m.ID != tt.wantID || m.Width != tt.wantWidth || m.Height != tt.wantHeight {
				t.Fatalf("map = %+v", m)
			}
			if len(layers) != 2 || layers[0].Kind != protocol.LayerTerrain || layers[1].Kind != protocol.LayerObjects {
				t.Fatalf("layers = %+v", layers)
			}
			if len(layers[0].Cells) != tt.wantWidth*tt.wantHeight {
				t.Fatalf("terrain cells = %d", len(layers[0].Cells))
			}
			warps := 0
			for _, cell := range layers[1].Cells {
				if cell.Kind == protocol.TileWarp {
					warps++
				}
			}
			if warps != tt.wantWarps {
				t.Fatalf("semantic warps = %d, want %d", warps, tt.wantWarps)
			}
		})
	}
}

func TestSnapshotUsesLiveGeometryPlayerAndEntities(t *testing.T) {
	producer := loadRedProducer(t)
	header, err := rom.ParseMap(producer.rom, 0x00)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := rom.Blocks(producer.rom, header)
	if err != nil {
		t.Fatal(err)
	}

	var mem state.Mem
	mem[sym.CurMap] = header.ID
	mem[sym.CurMapWidth] = header.WidthBlocks
	mem[sym.CurMapHeight] = header.HeightBlocks
	mem[sym.XCoord] = 7
	mem[sym.YCoord] = 8
	mem[sym.SpritePlayerFacing] = byte(state.FacingRight)
	mem[sym.WalkCounter] = 4
	mem[sym.PlayerMovingDirection] = 0x01
	writeLiveBlocks(&mem, header, blocks)
	if len(header.Objects) == 0 {
		t.Fatal("Pallet Town has no static objects")
	}
	writeLiveSprite(&mem, 1, 10, 12, header.Objects[0].SpriteID, byte(state.FacingLeft))

	got, err := producer.Snapshot(memorySnapshot{mem: &mem}, protocol.FrameMeta{Frame: 99, Cycle: 1234})
	if err != nil {
		t.Fatal(err)
	}
	if got.Map == nil || got.Map.ID != "pallet town" || got.Map.Width != 20 || got.Map.Height != 18 {
		t.Fatalf("map = %+v", got.Map)
	}
	if got.Player == nil || got.Player.Position != (protocol.Position{X: 7, Y: 8}) || got.Player.Facing != protocol.DirectionRight {
		t.Fatalf("player = %+v", got.Player)
	}
	if got.Player.Movement == nil || got.Player.Movement.To != (protocol.Position{X: 8, Y: 8}) {
		t.Fatalf("movement = %+v", got.Player.Movement)
	}
	if len(got.Entities) != 1 || got.Entities[0].Position != (protocol.Position{X: 10, Y: 12}) {
		t.Fatalf("entities = %+v", got.Entities)
	}
	if !got.HasCapability(protocol.CapabilityLayers) || !got.HasCapability(protocol.CapabilityEntities) {
		t.Fatalf("capabilities = %v", got.Capabilities)
	}
	if got.Clock.Frame != 99 || got.Clock.Cycle != 1234 {
		t.Fatalf("clock = %+v", got.Clock)
	}
}

func loadRedProducer(t *testing.T) *Producer {
	t.Helper()
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	producer, err := New(data)
	if err != nil {
		t.Fatal(err)
	}
	return producer
}

func writeLiveSprite(mem *state.Mem, slot, x, y int, pictureID, facing byte) {
	data1 := sym.SpritePlayerStateData1 + uint16(slot)*0x10
	data2 := sym.SpriteStateData2 + uint16(slot)*0x10
	mem[data1] = pictureID
	mem[data1+0x02] = 0
	mem[data1+0x09] = facing
	mem[data2+0x04] = byte(y + 4)
	mem[data2+0x05] = byte(x + 4)
}

func writeLiveBlocks(mem *state.Mem, header rom.MapHeader, blocks []byte) {
	widthBlocks, heightBlocks := int(header.WidthBlocks), int(header.HeightBlocks)
	stride := widthBlocks + 2*liveMapBorderBlocks
	first := liveMapBorderBlocks*stride + liveMapBorderBlocks
	for y := 0; y < heightBlocks; y++ {
		for x := 0; x < widthBlocks; x++ {
			off := first + y*stride + x
			mem[sym.OverworldMap+uint16(off)] = blocks[y*widthBlocks+x]
		}
	}
}
