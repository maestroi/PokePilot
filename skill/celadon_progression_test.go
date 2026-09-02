package skill

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

func TestCutRouteTile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tileset uint8
		tile    uint8
		want    bool
	}{
		{"overworld tree", overworldTileset, cutTreeTile, true},
		{"overworld gym-tree id", overworldTileset, gymCutTreeTile, false},
		{"gym tree", gymTileset, gymCutTreeTile, true},
		{"gym overworld-tree id", gymTileset, cutTreeTile, false},
		{"other tileset", 13, cutTreeTile, false},
		{"ordinary wall", overworldTileset, 0x2C, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cutRouteTile(tc.tileset, tc.tile); got != tc.want {
				t.Fatalf("cutRouteTile(%d,%#02x) = %v, want %v", tc.tileset, tc.tile, got, tc.want)
			}
		})
	}
}

func TestCutRecoverableNavigationError(t *testing.T) {
	for _, err := range []error{
		world.ErrNoPath,
		fmt.Errorf("wrapped: %w", world.ErrNoPath),
		ErrLegUnwalkable,
		fmt.Errorf("wrapped: %w", ErrLegUnwalkable),
		ErrReplanExhausted,
	} {
		if !cutRecoverableNavigationError(err) {
			t.Errorf("cutRecoverableNavigationError(%v) = false, want true", err)
		}
	}
	if cutRecoverableNavigationError(errors.New("trainer battle")) {
		t.Fatal("generic navigation/gameplay error was treated as a Cut-recoverable path failure")
	}
}

func TestCeladonOffersErikaChallenge(t *testing.T) {
	city, ok := GymAt(celadonCityMap)
	if !ok {
		t.Fatal("Celadon City has no gym challenge")
	}
	inside, ok := GymAt(celadonGymMap)
	if !ok {
		t.Fatal("Celadon Gym has no gym challenge")
	}
	if city.Leader != "ERIKA" || city.Badge != state.BadgeRainbow {
		t.Fatalf("city challenge = %+v, want Erika / Rainbow Badge", city)
	}
	if city != inside {
		t.Fatalf("city and interior challenges differ: city=%+v inside=%+v", city, inside)
	}
	if city.Place != "celadon gym" || city.LeaderX != 4 || city.LeaderY != 3 {
		t.Fatalf("Erika geometry = %+v, want celadon gym leader at (4,3)", city)
	}
}

func TestCeladonProgressionPlaces(t *testing.T) {
	want := map[string]Destination{
		"route 9":                    {Map: 0x14, X: 25, Y: 8},
		"route 10":                   {Map: 0x15, X: 11, Y: 20},
		"rock tunnel 1f":             {Map: 0x52, X: 15, Y: 4},
		"lavender town":              {Map: 0x04, X: 3, Y: 6},
		"route 8":                    {Map: 0x13, X: 13, Y: 4},
		"underground path route 8":   {Map: 0x50, X: 4, Y: 5},
		"underground path west east": {Map: 0x79, X: 46, Y: 2},
		"underground path route 7":   {Map: 0x4D, X: 4, Y: 5},
		"route 7":                    {Map: 0x12, X: 5, Y: 12},
		"celadon city":               {Map: 0x06, X: 41, Y: 10},
		"celadon gym":                {Map: 0x86, X: 4, Y: 4},
	}
	for name, dest := range want {
		got, ok := Place(name)
		if !ok {
			t.Errorf("Place(%q) missing", name)
			continue
		}
		if got != dest {
			t.Errorf("Place(%q) = %+v, want %+v", name, got, dest)
		}
	}
}
