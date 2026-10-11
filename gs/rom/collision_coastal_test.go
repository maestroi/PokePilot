package rom

import "testing"

// Every collision ID below is taken from the pinned pret/pokegold
// data/tilesets/*_collision.asm. The live routing table must preserve
// individual quadrants, not flatten walkability to a per-block boolean.
func TestCoastalNativeCollisionTablesMatchDecompQuadrants(t *testing.T) {
	for _, tc := range []struct {
		tileset uint8
		block   int
		want    [4]uint8
	}{
		{0x0d, 0x0c, [4]uint8{0x00, 0x00, 0x70, 0x70}}, // Game Corner / Olivine Café carpet
		{0x11, 0x0b, [4]uint8{0x72, 0x72, 0x00, 0x00}}, // Champions Room / Olivine Gym ladder
		{0x12, 0x09, [4]uint8{0x7a, 0x07, 0x00, 0x00}}, // Lighthouse stairs
		{0x12, 0x28, [4]uint8{0x00, 0x00, 0x60, 0x60}}, // Lighthouse pit/landing route
	} {
		table, ok := firstBadgeCollisionTable(tc.tileset)
		if !ok || len(table) != 64*4 {
			t.Fatalf("tileset %#02x collision length=%d ok=%t", tc.tileset, len(table), ok)
		}
		var got [4]uint8
		copy(got[:], table[tc.block*4:tc.block*4+4])
		if got != tc.want {
			t.Fatalf("tileset %#02x block %#02x quadrants = %#v, want %#v", tc.tileset, tc.block, got, tc.want)
		}
	}
}

func TestCoastalNativeTilesetMappingsMatchGoldMapHeaders(t *testing.T) {
	for _, tc := range []struct {
		name string
		want uint8
	}{
		{"OLIVINE_CAFE", 0x0d},
		{"OLIVINE_GYM", 0x11},
		{"OLIVINE_LIGHTHOUSE_1F", 0x12},
		{"OLIVINE_LIGHTHOUSE_6F", 0x12},
		{"OLIVINE_CITY", 0x01},
		{"ROUTE_40", 0x01},
		{"ROUTE_41", 0x01},
		{"CIANWOOD_PHARMACY", 0x04},
		{"CIANWOOD_GYM", 0x14},
		{"CIANWOOD_POKECENTER_1F", 0x06},
	} {
		got, ok := firstBadgeTileset(tc.name)
		if !ok || got != tc.want {
			t.Errorf("%s tileset = %#02x ok=%t, want %#02x", tc.name, got, ok, tc.want)
		}
	}
}

func TestCoastalNativeGridReadsNewCollisionTables(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	for _, tc := range []struct {
		name  string
		block byte
	}{
		{"OLIVINE_CAFE", 0x01},
		{"OLIVINE_GYM", 0x10},
		{"OLIVINE_LIGHTHOUSE_2F", 0x04},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := nativeID(t, tc.name)
			header, err := provider.ParseMap(id)
			if err != nil {
				t.Fatal(err)
			}
			blocks := make([]byte, int(header.WidthBlocks)*int(header.HeightBlocks))
			blocks[0] = tc.block
			grid, err := provider.Grid(id, blocks, 0)
			if err != nil {
				t.Fatal(err)
			}
			if !grid.Walkable[0] || !grid.Walkable[1] || !grid.Walkable[grid.Width] || !grid.Walkable[grid.Width+1] {
				t.Fatalf("%s known FLOOR block is not fully walkable", tc.name)
			}
			if grid.Walkable[2] {
				t.Fatalf("%s block-zero sentinel unexpectedly walkable", tc.name)
			}
		})
	}
}
