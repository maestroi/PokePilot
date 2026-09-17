package skill

import (
	"os"
	"testing"
)

// Yellow's WildDataPointers moved to 03:4B95 and Tilesets to 03:4558. Both
// formats are identical to Red's (the engine files diff clean), so if the
// resolver picks Yellow's tables, the readers must produce sensible results
// on a Yellow ROM: real grass rates on routes with grass, zero on towns.
func TestYellowWildTables(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	if !isYellowROM(romData) {
		t.Fatal("isYellowROM is false on the Yellow ROM; resolver is broken")
	}
	ts := tablesForROM(romData)
	if ts != yellowRomTables() {
		t.Fatalf("tablesForROM = %+v, want Yellow's", ts)
	}
	t.Logf("resolver: tilesets=%02x:%04x wild=%02x:%04x",
		ts.tilesetsBank, ts.tilesetsAddr, ts.wildBank, ts.wildAddr)

	// Route 1 (0x0C) must have grass encounters; Pallet Town (0x00) must not.
	cases := []struct {
		id        uint8
		name      string
		wantRate  uint8
		wantSlots bool
	}{
		{0x0C, "ROUTE_1", 25, true},
		{0x00, "PALLET_TOWN", 0, false},
	}
	for _, c := range cases {
		rate, err := wildGrassRate(romData, c.id)
		if err != nil {
			t.Errorf("wildGrassRate(0x%02x %s): %v", c.id, c.name, err)
			continue
		}
		if rate != c.wantRate {
			t.Errorf("wildGrassRate(0x%02x %s) = %d, want %d", c.id, c.name, rate, c.wantRate)
		}
		slots, err := WildGrassSlots(romData, c.id)
		if err != nil {
			t.Errorf("WildGrassSlots(0x%02x %s): %v", c.id, c.name, err)
			continue
		}
		if c.wantSlots && len(slots) == 0 {
			t.Errorf("WildGrassSlots(0x%02x %s) returned no slots", c.id, c.name)
		}
		if !c.wantSlots && len(slots) != 0 {
			t.Errorf("WildGrassSlots(0x%02x %s) = %d slots, want 0", c.id, c.name, len(slots))
		}
		t.Logf("%s: rate=%d slots=%d", c.name, rate, len(slots))
	}

	// WildGrass must return named species on Route 1.
	if wild, err := WildGrass(romData, 0x0C); err == nil && len(wild) > 0 {
		t.Logf("Route 1 wild: %d species", len(wild))
	} else if err != nil {
		t.Errorf("WildGrass(0x0C): %v", err)
	}
}
