package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// The retail map graph has a Route 44/Blackthorn overworld connection, but
// Ice Path remains an independent dungeon corridor with real pits and ladders.
func TestLateJohtoNativeTopologyRoutesIcePathAndBlackthornServices(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{
		{"MAHOGANY_TOWN", "ROUTE_44", "ICE_PATH_1F"},
		{"ICE_PATH_1F", "ICE_PATH_B1F"},
		{"ICE_PATH_B1F", "ICE_PATH_B2F_MAHOGANY_SIDE"},
		{"ICE_PATH_B2F_MAHOGANY_SIDE", "ICE_PATH_B3F"},
		{"ICE_PATH_B3F", "ICE_PATH_B2F_BLACKTHORN_SIDE"},
		{"ICE_PATH_1F", "BLACKTHORN_CITY"},
		{"BLACKTHORN_CITY", "BLACKTHORN_POKECENTER_1F"},
		{"BLACKTHORN_CITY", "BLACKTHORN_GYM_1F", "BLACKTHORN_GYM_2F"},
		{"BLACKTHORN_CITY", "DRAGONS_DEN_1F", "DRAGONS_DEN_B1F"},
	} {
		route, err := world.FindNativeRoute(graph, nativeID(t, names[0]), nativeID(t, names[len(names)-1]))
		if err != nil {
			t.Fatalf("late Johto native route %v: %v", names, err)
		}
		if len(route) != len(names)-1 {
			t.Fatalf("late Johto route %v uses %d legs, want %d: %+v", names, len(route), len(names)-1, route)
		}
		for i, edge := range route {
			if edge.From != nativeID(t, names[i]) || edge.To != nativeID(t, names[i+1]) {
				t.Fatalf("late Johto route %v leg %d = %+v", names, i, edge)
			}
		}
	}
}

func TestLateJohtoNativeGridUsesRealIcePathTiles(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	for _, name := range []string{
		"ROUTE_44", "ICE_PATH_1F", "ICE_PATH_B1F",
		"ICE_PATH_B2F_MAHOGANY_SIDE", "ICE_PATH_B2F_BLACKTHORN_SIDE",
		"ICE_PATH_B3F", "BLACKTHORN_CITY", "BLACKTHORN_POKECENTER_1F",
		"BLACKTHORN_MART", "BLACKTHORN_GYM_1F", "BLACKTHORN_GYM_2F",
		"DRAGONS_DEN_1F", "DRAGONS_DEN_B1F",
	} {
		t.Run(name, func(t *testing.T) {
			header, err := provider.ParseMap(nativeID(t, name))
			if err != nil {
				t.Fatal(err)
			}
			blocks := make([]byte, int(header.WidthBlocks)*int(header.HeightBlocks))
			if _, err := provider.Grid(header.ID, blocks, 0); err != nil {
				t.Fatalf("%s live tileset grid: %v", name, err)
			}
		})
	}
	for _, tileset := range []uint8{0x18, 0x19, 0x1a} {
		table, ok := firstBadgeCollisionTable(tileset)
		if !ok || len(table) != 256 {
			t.Fatalf("late Johto tileset %#02x has %d collision bytes (supported=%v), want 256", tileset, len(table), ok)
		}
	}
}

func TestLateJohtoIcePathPitWarpRetainsDestinationID(t *testing.T) {
	header, err := NewFirstBadgeWorldProvider(nil).ParseMap(nativeID(t, "ICE_PATH_B1F"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, warp := range header.Warps {
		if warp.X == 11 && warp.Y == 2 && warp.DestMap == nativeID(t, "ICE_PATH_B2F_MAHOGANY_SIDE") && warp.DestWarpID == 2 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Ice Path B1F hole warp (11,2) does not lead to Mahogany-side B2F warp 3")
	}
}

// HM07 Waterfall on Ice Path 1F is at (31,7). South of the ball is a wall, so
// the interaction stand must be west at (30,7). Staging on (31,8) made the
// Blackthorn-side ladder arrival (37,5) report world.ErrNoPath
// (run-a018a6cmhnkr / triage:3ef7c2a0a59e13a1).
func TestLateJohtoIcePathHM07StandIsReachableFromBlackthornLadder(t *testing.T) {
	rom := loadGold(t)
	mapID := nativeID(t, "ICE_PATH_1F")
	blocks, err := mapBlockData(rom, mapID)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := NewFirstBadgeWorldProvider(nil).Grid(mapID, blocks, worldmodel.TraversalLand)
	if err != nil {
		t.Fatal(err)
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Walkable(31, 8) {
		t.Fatal("south of HM07 (31,8) must remain a wall; south staging is invalid")
	}
	if !grid.Walkable(30, 7) || !grid.Walkable(31, 7) {
		t.Fatalf("HM07 west stand / item tiles not walkable: stand=%v item=%v",
			grid.Walkable(30, 7), grid.Walkable(31, 7))
	}
	path, err := world.FindNativePath(grid, 37, 5, 30, 7, nil)
	if err != nil || len(path) == 0 {
		t.Fatalf("Blackthorn ladder (37,5) -> HM07 stand (30,7): path=%d err=%v", len(path), err)
	}
	if _, err := world.FindNativePath(grid, 37, 5, 31, 8, nil); err == nil {
		t.Fatal("unexpected path onto wall staging tile (31,8)")
	}
}
