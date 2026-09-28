package rom

import (
	"testing"

	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

func collisionMapID(t *testing.T, name string) uint16 {
	t.Helper()
	info, ok := gsdata.MapByName(name)
	if !ok {
		t.Fatalf("unknown generated map %q", name)
	}
	return gsdata.NativeMapID(info.Group, info.Number)
}

func blocksFor(t *testing.T, mapID uint16, fill byte) []byte {
	t.Helper()
	info, ok := gsdata.Map(mapID)
	if !ok {
		t.Fatalf("unknown map %#04x", mapID)
	}
	blocks := make([]byte, int(info.WidthBlocks)*int(info.HeightBlocks))
	for i := range blocks {
		blocks[i] = fill
	}
	return blocks
}

func TestGen2GridDecodesCollisionQuadrants(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	mapID := collisionMapID(t, "PLAYERS_HOUSE_2F")
	blocks := blocksFor(t, mapID, 0x05)
	blocks[0] = 0x02 // WALL, STAIRCASE, FLOOR, FLOOR

	spec, err := provider.Grid(mapID, blocks, worldmodel.TraversalLand)
	if err != nil {
		t.Fatalf("Grid: %v", err)
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	if grid.Walkable(0, 0) {
		t.Fatal("top-left WALL decoded walkable")
	}
	if !grid.Walkable(1, 0) || !grid.Walkable(0, 1) || !grid.Walkable(1, 1) {
		t.Fatal("STAIRCASE/FLOOR quadrants should be walkable")
	}
}

func TestGen2GridTreatsBlockZeroAsCollisionFF(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	mapID := collisionMapID(t, "NEW_BARK_TOWN")
	blocks := blocksFor(t, mapID, 0x01)
	blocks[0] = 0

	spec, err := provider.Grid(mapID, blocks, worldmodel.TraversalLand)
	if err != nil {
		t.Fatalf("Grid: %v", err)
	}
	if spec.CollisionTile[0] != 0xff || spec.Walkable[0] {
		t.Fatalf("block zero tile = collision %#02x walkable=%v, want ff/false", spec.CollisionTile[0], spec.Walkable[0])
	}
}

func TestGen2GridLandAndWaterPermissions(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	mapID := collisionMapID(t, "NEW_BARK_TOWN")
	blocks := blocksFor(t, mapID, 0x35) // four COLL_WATER tiles

	land, err := provider.Grid(mapID, blocks, worldmodel.TraversalLand)
	if err != nil {
		t.Fatalf("land Grid: %v", err)
	}
	water, err := provider.Grid(mapID, blocks, worldmodel.TraversalWater)
	if err != nil {
		t.Fatalf("water Grid: %v", err)
	}
	if land.Walkable[0] {
		t.Fatal("water tile walkable on land")
	}
	if !water.Walkable[0] {
		t.Fatal("water tile not walkable while surfing")
	}
}

func TestGen2GridMarksCutTree(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	mapID := collisionMapID(t, "NEW_BARK_TOWN")
	blocks := blocksFor(t, mapID, 0x01)
	blocks[0] = 0x5b // HEADBUTT_TREE, CUT_TREE, FLOOR, FLOOR

	spec, err := provider.Grid(mapID, blocks, worldmodel.TraversalLand)
	if err != nil {
		t.Fatalf("Grid: %v", err)
	}
	if !spec.Cuttable[1] || spec.Walkable[1] {
		t.Fatalf("cut tree = cuttable %v walkable %v, want true/false", spec.Cuttable[1], spec.Walkable[1])
	}
}

func TestGen2NativeGridJumpsLedgeOnlyInAllowedDirection(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	mapID := collisionMapID(t, "NEW_BARK_TOWN")
	blocks := blocksFor(t, mapID, 0x01)
	blocks[0] = 0x4b // HOP_DOWN, FLOOR, WALL, FLOOR

	spec, err := provider.Grid(mapID, blocks, worldmodel.TraversalLand)
	if err != nil {
		t.Fatalf("Grid: %v", err)
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	move, ok := grid.Movement(0, 0, world.NativeStep{DY: 1}, nil)
	if !ok || move.DX != 0 || move.DY != 2 {
		t.Fatalf("down ledge move = %+v ok=%v, want (0,2)", move, ok)
	}
	if _, ok := grid.Movement(0, 0, world.NativeStep{DX: 1}, nil); !ok {
		t.Fatal("ordinary right step from ledge tile should remain legal")
	}
}

func TestGen2NativeGridRespectsDirectionalWallBothWays(t *testing.T) {
	spec := worldmodel.NativeGridSpec{
		MapID: 1, Width: 2, Height: 1,
		Walkable:      []bool{true, true},
		CollisionTile: []uint8{0xb0, 0x00}, // RIGHT_WALL, FLOOR
		Blocked:       gen2BlockedDirections(),
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	if grid.Passable(0, 0, 1, 0) {
		t.Fatal("RIGHT_WALL allowed movement to the right")
	}

	spec.CollisionTile = []uint8{0x00, 0xb1} // FLOOR, LEFT_WALL
	grid, err = world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec target wall: %v", err)
	}
	if grid.Passable(0, 0, 1, 0) {
		t.Fatal("entering LEFT_WALL from the left was allowed")
	}
}

func TestGen2SecondBadgeTilesetsDecodeCaveAndKurtHouse(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)

	for _, tc := range []struct {
		name  string
		block byte
	}{
		{name: "UNION_CAVE_1F", block: 0x02},
		{name: "SLOWPOKE_WELL_B1F", block: 0x02},
		{name: "KURTS_HOUSE", block: 0x04},
		{name: "AZALEA_GYM", block: 0x01},
		{name: "AZALEA_POKECENTER_1F", block: 0x04},
	} {
		mapID := collisionMapID(t, tc.name)
		blocks := blocksFor(t, mapID, tc.block)
		spec, err := provider.Grid(mapID, blocks, worldmodel.TraversalLand)
		if err != nil {
			t.Fatalf("%s Grid: %v", tc.name, err)
		}
		if len(spec.Walkable) == 0 || !spec.Walkable[0] {
			t.Fatalf("%s representative floor block decoded non-walkable", tc.name)
		}
	}
}
