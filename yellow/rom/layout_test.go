package rom

import "testing"

func TestYellowMapInventory(t *testing.T) {
	ids := MapIDs()
	if yellowPlayableMapCount != 227 {
		t.Fatalf("generated playable map constant = %d, want 227", yellowPlayableMapCount)
	}
	if len(ids) != yellowPlayableMapCount {
		t.Fatalf("playable map count = %d, want %d", len(ids), yellowPlayableMapCount)
	}
	for _, tc := range []struct {
		id   uint8
		name string
	}{
		{0x00, "PALLET_TOWN"},
		{0x26, "REDS_HOUSE_2F"},
		{0xF8, "SUMMER_BEACH_HOUSE"},
	} {
		if got := MapName(tc.id); got != tc.name {
			t.Errorf("MapName(%02x) = %q, want %q", tc.id, got, tc.name)
		}
	}
	for _, id := range []uint8{0x0B, 0x69, 0x6A, 0x6B, 0xED, 0xF4} {
		if validMapID(id) {
			t.Errorf("unused map %02x marked playable", id)
		}
	}
}

func TestYellowDuplicateMapsShareCanonicalHeaders(t *testing.T) {
	for _, tc := range []struct{ copy, live uint8 }{
		{0x45, 0x3E}, // CERULEAN_TRASHED_HOUSE_COPY
		{0x4B, 0x4A}, // UNDERGROUND_PATH_ROUTE_6_COPY
		{0x4E, 0x4D}, // UNDERGROUND_PATH_ROUTE_7_COPY
		{0xAD, 0xAC}, // CINNABAR_MART_COPY
	} {
		if yellowHeaderRefs[tc.copy] != yellowHeaderRefs[tc.live] {
			t.Errorf("copy %02x header = %+v, live %02x = %+v",
				tc.copy, yellowHeaderRefs[tc.copy], tc.live, yellowHeaderRefs[tc.live])
		}
	}
}

func TestYellowLayoutIncludesBeachHouseTileset(t *testing.T) {
	if yellowTilesetsAddr != 0x4558 || yellowTilesetsBank != 0x03 {
		t.Fatalf("Tilesets = %02x:%04x, want 03:4558", yellowTilesetsBank, yellowTilesetsAddr)
	}
}
