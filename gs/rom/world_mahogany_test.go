package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

// The executable seventh-badge storyline is a separate milestone. Pinning the
// physical approach prevents a fake Ecruteak-to-Mahogany shortcut and makes
// the Lake of Rage route available to the eventual scripted controller.
func TestMahoganyNativeTopologyRoutesAcrossRealJohtoConnections(t *testing.T) {
	graph, err := world.BuildNativeGraph(NewFirstBadgeWorldProvider(nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{
		{"ECRUTEAK_CITY", "ROUTE_42", "MAHOGANY_TOWN"},
		{"MAHOGANY_TOWN", "ROUTE_42", "ECRUTEAK_CITY"},
		{"MAHOGANY_TOWN", "ROUTE_43", "LAKE_OF_RAGE"},
		{"LAKE_OF_RAGE", "ROUTE_43", "MAHOGANY_TOWN"},
		{"MAHOGANY_TOWN", "MAHOGANY_POKECENTER_1F"},
		{"ROUTE_42_ECRUTEAK_GATE", "ROUTE_42"},
		{"MAHOGANY_TOWN", "ROUTE_43_MAHOGANY_GATE"},
		{"ROUTE_43_MAHOGANY_GATE", "ROUTE_43"},
	} {
		route, err := world.FindNativeRoute(graph, nativeID(t, names[0]), nativeID(t, names[len(names)-1]))
		if err != nil {
			t.Fatalf("Mahogany route %v: %v", names, err)
		}
		if len(route) != len(names)-1 {
			t.Fatalf("Mahogany route %v has %d legs, want %d: %+v", names, len(route), len(names)-1, route)
		}
		for i, edge := range route {
			if edge.From != nativeID(t, names[i]) || edge.To != nativeID(t, names[i+1]) {
				t.Fatalf("Mahogany route %v leg %d: %+v", names, i, edge)
			}
		}
	}
}

func TestMahoganyNativeTopologyHasCollisionForApproachAndHealing(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	for _, name := range []string{
		"ROUTE_42_ECRUTEAK_GATE", "ROUTE_42",
		"MAHOGANY_TOWN", "MAHOGANY_POKECENTER_1F",
		"ROUTE_43_MAHOGANY_GATE", "ROUTE_43", "LAKE_OF_RAGE",
	} {
		t.Run(name, func(t *testing.T) {
			header, err := provider.ParseMap(nativeID(t, name))
			if err != nil {
				t.Fatal(err)
			}
			blocks := make([]byte, int(header.WidthBlocks)*int(header.HeightBlocks))
			if _, err := provider.Grid(header.ID, blocks, 0); err != nil {
				t.Fatalf("Grid(%s): %v", name, err)
			}
		})
	}
}

func TestMahoganyNativeWarpsRetainRealDecompDestinationIDs(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	for _, tc := range []struct {
		from          string
		x, y          uint8
		to            string
		zeroBasedWarp uint8
	}{
		{"ROUTE_42_ECRUTEAK_GATE", 9, 4, "ROUTE_42", 0},
		{"ROUTE_42", 0, 8, "ROUTE_42_ECRUTEAK_GATE", 2},
		{"MAHOGANY_TOWN", 15, 13, "MAHOGANY_POKECENTER_1F", 0},
		{"MAHOGANY_POKECENTER_1F", 3, 7, "MAHOGANY_TOWN", 3},
		{"ROUTE_43_MAHOGANY_GATE", 4, 7, "MAHOGANY_TOWN", 4},
	} {
		header, err := provider.ParseMap(nativeID(t, tc.from))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, warp := range header.Warps {
			if warp.X == tc.x && warp.Y == tc.y && warp.DestMap == nativeID(t, tc.to) && warp.DestWarpID == tc.zeroBasedWarp {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s warp (%d,%d) does not lead to %s warp %d", tc.from, tc.x, tc.y, tc.to, tc.zeroBasedWarp)
		}
	}
}

func TestMahoganyGymAndRocketInteriorsRemainOutsideLiveTopology(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	for _, name := range []string{"MAHOGANY_GYM", "MAHOGANY_MART_1F", "TEAM_ROCKET_BASE_B1F"} {
		if _, err := provider.ParseMap(nativeID(t, name)); err == nil {
			t.Fatalf("%s is not ready for executable seventh-badge progression", name)
		}
	}
}
