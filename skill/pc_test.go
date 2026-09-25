package skill

import (
	"os"
	"testing"
)

func TestKnownPokemonCenterMapIncludesRegisteredCenters(t *testing.T) {
	for _, name := range []string{"viridian pokemon center", "pewter pokemon center", "cerulean pokemon center", "fuchsia pokemon center"} {
		d, ok := Place(name)
		if !ok {
			t.Fatalf("Place(%q) missing", name)
		}
		if !knownPokemonCenterMap(d.Map) {
			t.Fatalf("knownPokemonCenterMap(%#04x) = false for %s", d.Map, name)
		}
	}
}

func TestNearestPokemonCenterFromRoute16FlyHouseUsesNeighbour(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	// Direct component-aware routing from the Fly house cannot reach any
	// Center; the one-hop neighbour fallback must still name one so Travel
	// can restage through the Route 16 upper gate.
	center, name, err := nearestPokemonCenter(romData, route16FlyHouseMap)
	if err != nil {
		t.Fatalf("nearestPokemonCenter from Fly house: %v", err)
	}
	if !knownPokemonCenterMap(center.Map) {
		t.Fatalf("nearest center %q = %+v is not a known Pokemon Center", name, center)
	}
}
