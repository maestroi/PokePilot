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
		ErrNavigationStalled,
		// run-1948e1rnco3sp1y9bbhdwp7eov: from just inside Route 9 with the
		// tree still uncut, the capability-aware router can only find routes
		// that leave and re-enter through the same border crossing, and GoTo
		// walks that in a circle until this exact-position repeat guard
		// fires. It is the same "stuck here, tree in the way" signal
		// ErrNoPath already triggers Cut recovery for.
		fmt.Errorf("skill: GoTo: %w", ErrNavigationStalled),
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
		"route 9":                    {Map: 0x14, X: 25, Y: 8, Kind: DestinationMap},
		"route 10":                   {Map: 0x15, X: 11, Y: 20, Kind: DestinationMap},
		"rock tunnel 1f":             {Map: 0x52, X: 15, Y: 4, Kind: DestinationMap},
		"lavender town":              {Map: 0x04, X: 3, Y: 6, Kind: DestinationMap},
		"lavender pokemon center":    {Map: 0x8D, X: 3, Y: 3},
		"route 8":                    {Map: 0x13, X: 13, Y: 4, Kind: DestinationMap},
		"underground path route 8":   {Map: 0x50, X: 4, Y: 5},
		"underground path west east": {Map: 0x79, X: 46, Y: 2},
		"underground path route 7":   {Map: 0x4D, X: 4, Y: 5},
		"route 7":                    {Map: 0x12, X: 5, Y: 14, Kind: DestinationMap},
		"celadon city":               {Map: 0x06, X: 41, Y: 10, Kind: DestinationMap},
		"celadon pokemon center":     {Map: 0x85, X: 3, Y: 3},
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

func TestPostSurgeCeladonTravelStagesAtLavender(t *testing.T) {
	lavender, ok := Place("lavender town")
	if !ok {
		t.Fatal(`Place("lavender town") missing`)
	}
	center, ok := Place("celadon pokemon center")
	if !ok {
		t.Fatal(`Place("celadon pokemon center") missing`)
	}

	var got []Destination
	err := travelPostSurgeCeladon(0x03, func(dest Destination) (TravelResult, error) {
		got = append(got, dest)
		return TravelResult{}, nil
	})
	if err != nil {
		t.Fatalf("travelPostSurgeCeladon: %v", err)
	}
	if len(got) != 2 || got[0] != lavender || got[1] != center {
		t.Fatalf("travel legs = %+v, want Lavender then Celadon Center", got)
	}
}

// Issue #330 failed on Route 8 (map 0x13) after the old single TravelFlee call
// had accumulated ten legitimate dialogue recoveries since Cerulean. A retry
// from there must keep going west; walking back to Lavender merely to reset the
// counter would turn the semantic objective into a backtracking loop.
func TestPostSurgeCeladonTravelResumeAfterLavenderSkipsBacktrack(t *testing.T) {
	center, ok := Place("celadon pokemon center")
	if !ok {
		t.Fatal(`Place("celadon pokemon center") missing`)
	}

	var got []Destination
	err := travelPostSurgeCeladon(0x13, func(dest Destination) (TravelResult, error) {
		got = append(got, dest)
		return TravelResult{}, nil
	})
	if err != nil {
		t.Fatalf("travelPostSurgeCeladon: %v", err)
	}
	if len(got) != 1 || got[0] != center {
		t.Fatalf("travel legs = %+v, want only Celadon Center from Route 8", got)
	}
}

func TestPostSurgeCeladonTravelPreservesLegFailure(t *testing.T) {
	boom := errors.New("route failed")
	calls := 0
	err := travelPostSurgeCeladon(0x03, func(dest Destination) (TravelResult, error) {
		calls++
		return TravelResult{}, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped leg failure", err)
	}
	if calls != 1 {
		t.Fatalf("travel called %d times, want 1 after first-leg failure", calls)
	}
}
