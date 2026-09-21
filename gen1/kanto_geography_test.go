package gen1

import "testing"

func TestPostSurgeCeladonArea(t *testing.T) {
	for _, tc := range []struct {
		mapID uint8
		name  string
		want  bool
	}{
		{0x06, "CELADON_CITY", true},
		{0x85, "GAME_CORNER", true},
		{0xc7, "ROCKET_HIDEOUT_B4F", true},
		{0x04, "LAVENDER_TOWN", false},
	} {
		if got := PostSurgeCeladonArea(tc.mapID, tc.name); got != tc.want {
			t.Errorf("PostSurgeCeladonArea(%#02x,%q)=%v want %v", tc.mapID, tc.name, got, tc.want)
		}
	}
}

func TestPostSurgeLavenderReached(t *testing.T) {
	for _, tc := range []struct {
		mapID uint8
		name  string
		want  bool
	}{
		{0x04, "LAVENDER_TOWN", true},
		{0x79, "UNDERGROUND_PATH_WEST_EAST", true},
		{0xa5, "POKEMON_TOWER_3F", true},
		{0x0a, "SAFFRON_CITY", true},
		{0x06, "CELADON_CITY", true},
		{0x03, "CERULEAN_CITY", false},
	} {
		if got := PostSurgeLavenderReached(tc.mapID, tc.name); got != tc.want {
			t.Errorf("PostSurgeLavenderReached(%#02x,%q)=%v want %v", tc.mapID, tc.name, got, tc.want)
		}
	}
}
