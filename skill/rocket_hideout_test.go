package skill

import "testing"

func TestRocketHideoutAvailable(t *testing.T) {
	for _, mapID := range []uint8{
		celadonCityMap,
		celadonPokemonCenterMap,
		gameCornerMap,
		rocketHideoutB1FMap,
		rocketHideoutB2FMap,
		rocketHideoutB3FMap,
		rocketHideoutB4FMap,
	} {
		if !RocketHideoutAvailable(mapID) {
			t.Errorf("RocketHideoutAvailable(%#04x) = false, want true", mapID)
		}
	}
	for _, mapID := range []uint8{0x00, 0x04, 0x86} {
		if RocketHideoutAvailable(mapID) {
			t.Errorf("RocketHideoutAvailable(%#04x) = true, want false", mapID)
		}
	}
}

func TestRocketHideoutConstants(t *testing.T) {
	if silphScopeItem != 0x48 {
		t.Fatalf("silphScopeItem = %#02x, want 0x48", silphScopeItem)
	}
	if gameCornerMap != 0x87 || rocketHideoutB1FMap != 0xC7 || rocketHideoutB4FMap != 0xCA {
		t.Fatalf("map ids = GameCorner %#02x B1F %#02x B4F %#02x, want 0x87 0xC7 0xCA",
			gameCornerMap, rocketHideoutB1FMap, rocketHideoutB4FMap)
	}
}

func TestReplacedBlockCells(t *testing.T) {
	tests := []struct {
		name           string
		blockY, blockX int
		want           [][2]int
	}{
		{
			name:   "game corner secret door",
			blockY: 2,
			blockX: 8,
			want:   [][2]int{{16, 4}, {17, 4}, {16, 5}, {17, 5}},
		},
		{
			name:   "B4F boss door",
			blockY: 5,
			blockX: 12,
			want:   [][2]int{{24, 10}, {25, 10}, {24, 11}, {25, 11}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replacedBlockCells(tt.blockY, tt.blockX)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("cell %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
