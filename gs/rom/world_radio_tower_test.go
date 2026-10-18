package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestRadioTowerNativeGraphReachesFakeDirectorAndWarehouse(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{
		{"GOLDENROD_CITY", "RADIO_TOWER_1F", "RADIO_TOWER_2F",
			"RADIO_TOWER_3F", "RADIO_TOWER_4F", "RADIO_TOWER_5F"},
		{"RADIO_TOWER_5F", "RADIO_TOWER_4F", "RADIO_TOWER_3F",
			"RADIO_TOWER_2F", "RADIO_TOWER_1F", "GOLDENROD_CITY"},
		{"GOLDENROD_CITY", "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES",
			"GOLDENROD_UNDERGROUND_WAREHOUSE"},
		// Farm triage 8ef4690e9488f69d: wrong tileset on the switch-room
		// entrances made every leave edge look unreachable, so native
		// routing from the city-exit carpet to the tunnel failed with
		// no_route. Pin the tunnel leg itself.
		{"GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", "GOLDENROD_UNDERGROUND"},
		{"GOLDENROD_CITY", "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES",
			"GOLDENROD_UNDERGROUND"},
	} {
		route, err := world.FindNativeRoute(graph, nativeID(t, names[0]), nativeID(t, names[len(names)-1]))
		if err != nil {
			t.Fatalf("%v: %v", names, err)
		}
		if len(route) != len(names)-1 {
			t.Fatalf("%v: got %d legs, want %d", names, len(route), len(names)-1)
		}
		for i, leg := range route {
			if leg.From != nativeID(t, names[i]) || leg.To != nativeID(t, names[i+1]) {
				t.Fatalf("%v leg %d = %+v", names, i, leg)
			}
		}
	}
}

func TestGoldenrodUndergroundRetailTilesets(t *testing.T) {
	// pokegold/data/maps/maps.asm: gate / elite-four-room / underground.
	for _, tc := range []struct {
		name    string
		tileset uint8
	}{
		{"GOLDENROD_UNDERGROUND", 0x07},
		{"GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", 0x0e},
		{"GOLDENROD_UNDERGROUND_WAREHOUSE", 0x19},
	} {
		got, ok := firstBadgeTileset(tc.name)
		if !ok || got != tc.tileset {
			t.Fatalf("%s tileset = %#02x ok=%v, want %#02x", tc.name, got, ok, tc.tileset)
		}
	}
	// City-exit carpet block on the switch-room map: under the wrong
	// UNDERGROUND table this was floor, so gen2WarpCollision never fired.
	table, ok := firstBadgeCollisionTable(0x0e)
	if !ok || len(table) < 0x1c*4+4 {
		t.Fatal("TILESET_ELITE_FOUR_ROOM collision table missing")
	}
	i := 0x1c * 4
	if table[i+2] != 0x70 || table[i+3] != 0x70 {
		t.Fatalf("elite-four block $1c = {%#02x,%#02x,%#02x,%#02x}, want carpet-down 70/70",
			table[i], table[i+1], table[i+2], table[i+3])
	}
}

func TestRadioTowerRetailWarpsAndCollisionArePinned(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	for _, name := range []string{
		"RADIO_TOWER_1F", "RADIO_TOWER_2F", "RADIO_TOWER_3F", "RADIO_TOWER_4F", "RADIO_TOWER_5F",
		"GOLDENROD_UNDERGROUND", "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES",
		"GOLDENROD_UNDERGROUND_WAREHOUSE",
	} {
		h, err := provider.ParseMap(nativeID(t, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.Grid(h.ID, make([]byte, int(h.WidthBlocks)*int(h.HeightBlocks)), 0); err != nil {
			t.Fatalf("%s collision: %v", name, err)
		}
	}
	for _, tc := range []struct {
		from string
		x, y uint8
		to   string
		warp uint8
	}{
		{"GOLDENROD_CITY", 5, 15, "RADIO_TOWER_1F", 0},
		{"RADIO_TOWER_1F", 15, 0, "RADIO_TOWER_2F", 1},
		{"RADIO_TOWER_3F", 7, 0, "RADIO_TOWER_4F", 1},
		{"RADIO_TOWER_4F", 0, 0, "RADIO_TOWER_5F", 0},
		{"RADIO_TOWER_5F", 12, 0, "RADIO_TOWER_4F", 2},
		{"GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", 22, 10, "GOLDENROD_UNDERGROUND_WAREHOUSE", 0},
		{"GOLDENROD_UNDERGROUND_WAREHOUSE", 17, 2, "GOLDENROD_DEPT_STORE_B1F", 0},
	} {
		h, err := provider.ParseMap(nativeID(t, tc.from))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, w := range h.Warps {
			if w.X == tc.x && w.Y == tc.y &&
				w.DestMap == nativeID(t, tc.to) && w.DestWarpID == tc.warp {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s (%d,%d) -> %s warp %d is absent", tc.from, tc.x, tc.y, tc.to, tc.warp)
		}
	}
	table, ok := firstBadgeCollisionTable(0x18)
	if !ok || len(table) != 256 || table[3*4+1] != 0x7a {
		t.Fatal("Radio Tower tileset must use real staircase collision")
	}
}
