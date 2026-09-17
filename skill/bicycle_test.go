package skill

import "testing"

func TestBicycleProgressionDestinations(t *testing.T) {
	tests := []struct {
		name        string
		mapID, x, y uint8
	}{
		{name: pokemonFanClubChairmanPlace, mapID: 0x5a, x: 3, y: 3},
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

func TestFanClubChairmanDestinationUsesCounterApproach(t *testing.T) {
	dest, ok := Place(pokemonFanClubChairmanPlace)
	if !ok {
		t.Fatalf("Place(%q) not registered", pokemonFanClubChairmanPlace)
	}
	if dest.X != fanClubChairmanX || dest.Y != fanClubChairmanY+2 {
		t.Fatalf("Fan Club chairman destination = (%d,%d), want counter approach two tiles below chairman (%d,%d)",
			dest.X, dest.Y, fanClubChairmanX, fanClubChairmanY+2)
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
