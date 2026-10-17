package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
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
