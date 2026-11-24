package agent

import (
	"os"
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/worldmodel"
)

// Every Kanto leader stand tile must be ROM floor next to the leader; these
// were hand-written and Janine's (1,11) was a wall.
func TestGSKantoGymStandTilesAreWalkableBesideLeaders(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	provider := gsprofile.New(gsprofile.GoldGameID).NativeMapProvider(rom)
	for _, spec := range gsKantoGymStages {
		id, err := gsOpeningMapID(spec.gym)
		if err != nil {
			t.Fatal(err)
		}
		grid, err := provider.Grid(id, nil, worldmodel.TraversalLand)
		if err != nil {
			t.Fatal(err)
		}
		dx, dy := int(spec.sx)-int(spec.x), int(spec.sy)-int(spec.y)
		if !grid.Walkable[int(spec.sy)*grid.Width+int(spec.sx)] || dx*dx+dy*dy != 1 {
			t.Errorf("%s: stand (%d,%d) is not ROM floor beside the leader at (%d,%d)",
				spec.leader, spec.sx, spec.sy, spec.x, spec.y)
		}
	}
}

// The Power Plant approach and Fan Club stand were hand-picked from ROM
// collision; pin land vs water so a slip cannot reroute into a wall.
func TestGSKantoWaypointsMatchROMCollision(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	provider := gsprofile.New(gsprofile.GoldGameID).NativeMapProvider(rom)
	for _, c := range []struct {
		mapName string
		x, y    int
		water   bool
	}{
		{"CERULEAN_CITY", 38, 22, false},
		{"ROUTE_9", 50, 4, false},
		{"ROUTE_9", 50, 3, true},
		{"ROUTE_9", 56, 16, true},
		{"POKEMON_FAN_CLUB", 1, 3, false},
	} {
		id, err := gsOpeningMapID(c.mapName)
		if err != nil {
			t.Fatal(err)
		}
		land, err := provider.Grid(id, nil, worldmodel.TraversalLand)
		if err != nil {
			t.Fatal(err)
		}
		sea, err := provider.Grid(id, nil, worldmodel.TraversalWater)
		if err != nil {
			t.Fatal(err)
		}
		i := c.y*land.Width + c.x
		if land.Walkable[i] == c.water || !sea.Walkable[i] {
			t.Errorf("%s (%d,%d): land=%v water-mode=%v, want water=%v", c.mapName, c.x, c.y, land.Walkable[i], sea.Walkable[i], c.water)
		}
	}
}

// With the ROM topology, a Kanto map heals at a Kanto Center, never by
// routing back to Johto.
func TestGSKantoHealsAtNearestKantoCenter(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for from, want := range map[string]string{
		"CERULEAN_CITY":      "CERULEAN_POKECENTER_1F",
		"SILVER_CAVE_ROOM_1": "SILVER_CAVE_POKECENTER_1F",
		"VERMILION_CITY":     "VERMILION_POKECENTER_1F",
	} {
		id, _ := gsOpeningMapID(from)
		wantID, _ := gsOpeningMapID(want)
		got, err := gsOwnedPokecenterID(rom, id)
		if err != nil || got != wantID {
			t.Errorf("%s heals at %#04x (%v), want %s", from, got, err, want)
		}
	}
}
