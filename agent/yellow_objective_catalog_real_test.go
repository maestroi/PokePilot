package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/world"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

func TestRealYellowDestinationCatalogTargetsWalkableTiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Yellow ROM qualification is opt-in")
	}
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		path = "roms/pokemon_yellow.gb"
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("POKEMON_YELLOW_ROM: %v", err)
	}

	destinations, err := buildYellowDestinations(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(destinations) < 200 {
		t.Fatalf("Yellow destinations = %d, want broad playable-map coverage", len(destinations))
	}

	foundSummer := false
	for _, destination := range destinations {
		mapID, ok := yellowNativeMapForLocation(destination.Location)
		if !ok {
			t.Fatalf("location %q has no native Yellow map", destination.Location)
		}
		h, err := yellowrom.ParseMap(romData, mapID)
		if err != nil {
			t.Fatalf("%s: %v", destination.Place, err)
		}
		grid, err := world.Build(romData, h)
		if err != nil {
			t.Fatalf("%s grid: %v", destination.Place, err)
		}
		if !grid.Walkable(int(destination.X), int(destination.Y)) {
			t.Fatalf("%s target (%d,%d) is not walkable", destination.Place, destination.X, destination.Y)
		}
		if destination.Place == "summer beach house" {
			foundSummer = true
			if mapID != 0xf8 {
				t.Fatalf("Summer Beach House map = %#02x, want 0xf8", mapID)
			}
		}
	}
	if !foundSummer {
		t.Fatal("Summer Beach House missing from Yellow destination catalog")
	}
}
