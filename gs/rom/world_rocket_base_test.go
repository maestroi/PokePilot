package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

// Warps are the physical retail map graph, not plot completion. Doors,
// scripts and security cameras must be resolved by a separate owned
// objective before a runner may claim Rocket Base progression.
func TestRocketBaseNativeGraphExposesAllThreeFloorsAndPryceGym(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{
		{"MAHOGANY_TOWN", "MAHOGANY_MART_1F", "TEAM_ROCKET_BASE_B1F", "TEAM_ROCKET_BASE_B2F", "TEAM_ROCKET_BASE_B3F"},
		{"TEAM_ROCKET_BASE_B3F", "TEAM_ROCKET_BASE_B2F", "TEAM_ROCKET_BASE_B1F", "MAHOGANY_MART_1F", "MAHOGANY_TOWN"},
		{"MAHOGANY_TOWN", "MAHOGANY_GYM"},
	} {
		route, err := world.FindNativeRoute(graph, nativeID(t, names[0]), nativeID(t, names[len(names)-1]))
		if err != nil {
			t.Fatalf("%v: %v", names, err)
		}
		if len(route) != len(names)-1 {
			t.Fatalf("%v: %d legs, expected %d: %+v", names, len(route), len(names)-1, route)
		}
		for i, leg := range route {
			if leg.From != nativeID(t, names[i]) || leg.To != nativeID(t, names[i+1]) {
				t.Fatalf("route %v leg %d: %+v", names, i, leg)
			}
		}
	}
}

func TestRocketBaseWarpsUseZeroBasedRetailDestinationIDs(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	for _, tc := range []struct {
		from string
		x, y uint8
		to string
		warp uint8
	}{
		{"MAHOGANY_MART_1F", 7, 3, "TEAM_ROCKET_BASE_B1F", 0},
		{"TEAM_ROCKET_BASE_B1F", 27, 2, "MAHOGANY_MART_1F", 2},
		{"TEAM_ROCKET_BASE_B1F", 3, 14, "TEAM_ROCKET_BASE_B2F", 0},
		{"TEAM_ROCKET_BASE_B2F", 3, 14, "TEAM_ROCKET_BASE_B1F", 1},
		{"TEAM_ROCKET_BASE_B2F", 3, 2, "TEAM_ROCKET_BASE_B3F", 0},
		{"TEAM_ROCKET_BASE_B3F", 3, 2, "TEAM_ROCKET_BASE_B2F", 1},
		{"MAHOGANY_GYM", 4, 17, "MAHOGANY_TOWN", 2},
	} {
		header, err := provider.ParseMap(nativeID(t, tc.from))
		if err != nil {
			t.Fatal(err)
		}
		seen := false
		for _, warp := range header.Warps {
			if warp.X == tc.x && warp.Y == tc.y && warp.DestMap == nativeID(t, tc.to) && warp.DestWarpID == tc.warp {
				seen = true
			}
		}
		if !seen {
			t.Fatalf("%s (%d,%d) does not reach %s warp %d", tc.from, tc.x, tc.y, tc.to, tc.warp)
		}
	}
}

func TestRocketUndergroundCollisionSourcePreservesWallsAndWarpCarpets(t *testing.T) {
	table, ok := firstBadgeCollisionTable(0x19)
	if !ok || len(table) != 64*4 {
		t.Fatalf("TILESET_UNDERGROUND = %d collision bytes (ok=%v), want 256", len(table), ok)
	}
	for _, tc := range []struct {
		block uint8
		quadrant int
		want uint8
	}{
		{0x00, 0, 0x07}, // wall
		{0x02, 1, 0x72}, // ladder
		{0x07, 2, 0x60}, // pit
		{0x0f, 3, 0x7c}, // warp panel
		{0x24, 2, 0x70}, // carpet down
		{0x25, 1, 0x78}, // carpet up
		{0x27, 1, 0x7e}, // carpet right
		{0x28, 0, 0x76}, // carpet left
	} {
		if got := table[int(tc.block)*4+tc.quadrant]; got != tc.want {
			t.Fatalf("underground block %#02x quadrant %d = %#02x, want %#02x", tc.block, tc.quadrant, got, tc.want)
		}
	}
}
