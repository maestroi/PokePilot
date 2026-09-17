package rom

import (
	"os"
	"testing"
)

func testROM(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	return b
}

// TestParseKnownMaps decodes maps whose geometry is stated explicitly in
// pokeyellow/constants/map_constants.asm. Width/height/tileset are read from
// ROM, so this catches a wrong table address (the failure mode that made the
// Red parser reject 215 of 248 Yellow maps).
func TestParseKnownMaps(t *testing.T) {
	romData := testROM(t)
	cases := []struct {
		id            uint8
		name          string
		tileset       uint8
		width, height uint8
	}{
		{0x00, "PALLET_TOWN", 0, 10, 9},
		{0x01, "VIRIDIAN_CITY", 0, 20, 18},
		{0x02, "PEWTER_CITY", 0, 20, 18},
		{0x03, "CERULEAN_CITY", 0, 20, 18},
		{0x04, "LAVENDER_TOWN", 0, 10, 9},
		{0x26, "REDS_HOUSE_2F", 4, 4, 4},
	}
	for _, c := range cases {
		h, err := ParseMap(romData, c.id)
		if err != nil {
			t.Errorf("ParseMap(0x%02x %s): %v", c.id, c.name, err)
			continue
		}
		if h.Tileset != c.tileset || h.WidthBlocks != c.width || h.HeightBlocks != c.height {
			t.Errorf("ParseMap(0x%02x %s) = tileset %d, %dx%d; want %d, %dx%d",
				c.id, c.name, h.Tileset, h.WidthBlocks, h.HeightBlocks, c.tileset, c.width, c.height)
		}
	}
}

// TestParseTailMaps pins the ids where Yellow's valid-map set differs from
// Red's. Red's predicate rejects 0xF8+ outright; Yellow appends
// SUMMER_BEACH_HOUSE at 0xF8 and keeps AGATHAS_ROOM at 0xF7 (a real map in
// BOTH games). A regression that reuses Red's cutoff silently drops 0xF7, and
// one that copies Red's exclusion list drops 0xF8.
func TestParseTailMaps(t *testing.T) {
	romData := testROM(t)
	cases := []struct {
		id        uint8
		name      string
		tileset   uint8
		w, h      uint8
		shouldErr bool
	}{
		{0xF7, "AGATHAS_ROOM", 15, 5, 6, false},       // CEMETERY; real in both games
		{0xF8, "SUMMER_BEACH_HOUSE", 24, 7, 4, false}, // BEACH_HOUSE; appended in Yellow
		{0xF9, "", 0, 0, 0, true},                     // past the end
	}
	for _, c := range cases {
		got, err := ParseMap(romData, c.id)
		if c.shouldErr {
			if err == nil {
				t.Errorf("ParseMap(0x%02x) should fail but returned tileset %d", c.id, got.Tileset)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMap(0x%02x %s): %v", c.id, c.name, err)
			continue
		}
		if got.Tileset != c.tileset || got.WidthBlocks != c.w || got.HeightBlocks != c.h {
			t.Errorf("ParseMap(0x%02x %s) = tileset %d, %dx%d; want %d, %dx%d",
				c.id, c.name, got.Tileset, got.WidthBlocks, got.HeightBlocks, c.tileset, c.w, c.h)
		}
	}
}

// TestParseAllMaps pins the share of slots that decode. Yellow shares Red's
// header format, so the same structural expectations apply; the count is a
// regression guard against a table address drifting.
func TestParseAllMaps(t *testing.T) {
	romData := testROM(t)
	ok, invalid := 0, 0
	for id := uint8(0); id < 0xf9; id++ {
		if !validMapID(id) {
			continue
		}
		if _, err := ParseMap(romData, id); err != nil {
			invalid++
			continue
		}
		ok++
	}
	// Every Yellow map id in the valid range should decode: the tables parse
	// or the addresses are wrong. 227 = the 249 decomp map ids minus the 22
	// unused slots Yellow shares with Red.
	if invalid != 0 {
		t.Errorf("%d valid Yellow map ids failed to parse", invalid)
	}
	if ok != 227 {
		t.Errorf("parsed %d Yellow maps, want 227", ok)
	}
	t.Logf("parsed %d Yellow maps, %d failed", ok, invalid)
}

// TestWarpsMatchDecomp pins PalletTown's geometry against
// pokeyellow/data/maps/{headers,objects}/PalletTown.asm: 3 warps (to
// REDS_HOUSE_1F $25, BLUES_HOUSE $27, OAKS_LAB $28), 2 connections (north to
// ROUTE_1 $0C, south to ROUTE_21 $20) and 3 objects. It reads all of it from
// ROM, so a wrong table address or a misparsed object layout fails here.
func TestWarpsMatchDecomp(t *testing.T) {
	romData := testROM(t)
	h, err := ParseMap(romData, 0x00)
	if err != nil {
		t.Fatal(err)
	}
	wantWarps := map[[2]uint8]uint8{
		{5, 5}:   0x25, // REDS_HOUSE_1F
		{13, 5}:  0x27, // BLUES_HOUSE
		{12, 11}: 0x28, // OAKS_LAB
	}
	if len(h.Warps) != len(wantWarps) {
		t.Fatalf("PalletTown warps = %d, want %d", len(h.Warps), len(wantWarps))
	}
	for _, w := range h.Warps {
		want, ok := wantWarps[[2]uint8{w.X, w.Y}]
		if !ok {
			t.Errorf("unexpected warp at (%d,%d)", w.X, w.Y)
			continue
		}
		if w.DestMap != want {
			t.Errorf("warp at (%d,%d) -> map 0x%02x, want 0x%02x", w.X, w.Y, w.DestMap, want)
		}
	}
	wantConns := map[uint8]uint8{0: 0x0C, 1: 0x20} // dir: north/south -> map
	for _, c := range h.Connections {
		want, ok := wantConns[c.Dir]
		if !ok {
			t.Errorf("unexpected connection dir %d", c.Dir)
			continue
		}
		if c.MapID != want {
			t.Errorf("connection dir %d -> map 0x%02x, want 0x%02x", c.Dir, c.MapID, want)
		}
	}
	if len(h.Connections) != 2 {
		t.Errorf("PalletTown connections = %d, want 2", len(h.Connections))
	}
	if len(h.Objects) != 3 {
		t.Errorf("PalletTown objects = %d, want 3 (Oak, girl, fisher)", len(h.Objects))
	}
	if len(h.Signs) != 4 {
		t.Errorf("PalletTown signs = %d, want 4", len(h.Signs))
	}
}
