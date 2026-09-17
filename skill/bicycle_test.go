package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

func TestBicycleProgressionDestinations(t *testing.T) {
	tests := []struct {
		name        string
		mapID, x, y uint8
	}{
		{name: pokemonFanClubChairmanPlace, mapID: 0x5a, x: fanClubStagingX, y: fanClubStagingY},
		{name: ceruleanBikeShopPlace, mapID: 0x42, x: 3, y: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Place(tt.name)
			if !ok {
				t.Fatalf("Place(%q) not registered", tt.name)
			}
			if got.Map != tt.mapID || got.X != tt.x || got.Y != tt.y {
				t.Fatalf("Place(%q) = map %#04x (%d,%d), want map %#04x (%d,%d)", tt.name, got.Map, got.X, got.Y, tt.mapID, tt.x, tt.y)
			}
		})
	}
}

func TestFanClubChairmanDestinationStagesInsideEntrance(t *testing.T) {
	dest, ok := Place(pokemonFanClubChairmanPlace)
	if !ok {
		t.Fatalf("Place(%q) not registered", pokemonFanClubChairmanPlace)
	}
	if dest.X != 2 || dest.Y != 6 {
		t.Fatalf("Fan Club staging destination = (%d,%d), want open floor just inside entrance (2,6)", dest.X, dest.Y)
	}
	if dest.X == fanClubChairmanX && dest.Y > fanClubChairmanY && dest.Y <= fanClubChairmanY+2 {
		t.Fatalf("Fan Club destination (%d,%d) must not guess a straight-line chairman approach through the table", dest.X, dest.Y)
	}
}

// TestFanClubStagingRouteReachesChairman protects the geometry that the
// production failure exposed: entering POKEMON_FAN_CLUB lands at (2,7), while
// the large table blocks the naive straight-line destinations below the
// chairman. Bicycle progression should only route to open staging floor and
// leave the final approach to TalkAt, which finds a reachable adjacent side.
func TestFanClubStagingRouteReachesChairman(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM %s: %v", romPath, err)
	}
	h, err := rom.ParseMap(romData, pokemonFanClubMap)
	if err != nil {
		t.Fatalf("parse Pokemon Fan Club: %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("build Pokemon Fan Club grid: %v", err)
	}

	dest, ok := Place(pokemonFanClubChairmanPlace)
	if !ok {
		t.Fatalf("Place(%q) not registered", pokemonFanClubChairmanPlace)
	}
	if !grid.Walkable(int(dest.X), int(dest.Y)) {
		t.Fatalf("Fan Club staging destination (%d,%d) is not walkable", dest.X, dest.Y)
	}
	if _, err := world.FindPath(grid, 2, 7, int(dest.X), int(dest.Y), nil); err != nil {
		t.Fatalf("Fan Club entrance (2,7) cannot reach staging destination (%d,%d): %v", dest.X, dest.Y, err)
	}

	blocked := make(map[[2]int]bool)
	for _, object := range h.Objects {
		if object.X == fanClubChairmanX && object.Y == fanClubChairmanY {
			continue
		}
		blocked[[2]int{int(object.X), int(object.Y)}] = true
	}
	if _, _, err := world.FindPathAdjacent(grid, int(dest.X), int(dest.Y), int(fanClubChairmanX), int(fanClubChairmanY), blocked); err != nil {
		t.Fatalf("Fan Club staging destination (%d,%d) cannot reach a side of chairman (%d,%d): %v",
			dest.X, dest.Y, fanClubChairmanX, fanClubChairmanY, err)
	}
}

func TestBicycleProgressionUsesRedItemIDs(t *testing.T) {
	if bicycleItem != 0x06 {
		t.Fatalf("bicycle item = %#02x, want 0x06", bicycleItem)
	}
	if bikeVoucherItem != 0x2d {
		t.Fatalf("bike voucher item = %#02x, want 0x2d", bikeVoucherItem)
	}
}
