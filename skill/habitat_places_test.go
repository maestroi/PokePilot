package skill

import (
	"os"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestHabitatPlacesNameTheirMapLocation(t *testing.T) {
	for name, habitat := range habitatPlaces {
		mapID := habitat.Map
		want := strings.ToLower(strings.ReplaceAll(state.MapName(mapID), "_", " "))
		if name != want {
			t.Errorf("habitat place %q is map %#02x, whose location is %q", name, mapID, want)
		}
		if d, ok := Place(name); !ok || d.Map != mapID || d.Kind != DestinationMap {
			t.Errorf("Place(%q) = %+v, %v; want map arrival on %#02x", name, d, ok, mapID)
		}
	}
}

// TestEveryWildHabitatHasPlace is the invariant behind habitatPlaces: a map
// with ordinary wild encounters and no Place can never become a learned
// training area.
func TestEveryWildHabitatHasPlace(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	placed := map[uint8]bool{}
	for _, name := range PlaceNames() {
		if d, ok := Place(name); ok {
			placed[d.Map] = true
		}
	}
	for id := 0; id < 0xF8; id++ {
		mapID := uint8(id)
		name := state.MapName(mapID)
		// Safari Zone encounters are capture-only and cost an entry fee;
		// Pokemon Tower's ghosts cannot be fought before the Silph Scope.
		// Neither is ordinary training ground.
		if strings.HasPrefix(name, "SAFARI_ZONE_") || strings.HasPrefix(name, "POKEMON_TOWER_") {
			continue
		}
		grass, err := HasGrass(romData, mapID)
		if err != nil || !grass {
			continue
		}
		if !placed[mapID] {
			t.Errorf("wild habitat %#02x %s has no travel Place", mapID, name)
		}
	}
}
