package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
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

func TestBillsPCMenuScreenAcceptsYellowPrintBox(t *testing.T) {
	// Yellow inserts PRINT BOX before SEE YA!, raising MaxMenuItem from 4 to 5.
	// Recognition must accept both layouts while still requiring WITHDRAW/DEPOSIT.
	mem := changeBoxPromptMem("WITHDRAW", "DEPOSIT", "RELEASE", "CHANGE BOX", "PRINT BOX", "SEE YA!")
	mem[sym.TopMenuItemX] = 1
	mem[sym.TopMenuItemY] = 2

	mem[sym.MaxMenuItem] = 4
	if !billsPCMenuScreen(mem) {
		t.Fatalf("Red Bill's PC (max=4) not recognized: screen=%q", state.ScreenText(mem))
	}
	mem[sym.MaxMenuItem] = 5
	if !billsPCMenuScreen(mem) {
		t.Fatalf("Yellow Bill's PC with PRINT BOX (max=5) not recognized: screen=%q", state.ScreenText(mem))
	}
	mem[sym.MaxMenuItem] = 3
	if billsPCMenuScreen(mem) {
		t.Fatalf("accepted truncated Bill's PC menu: screen=%q", state.ScreenText(mem))
	}
}
