package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestCuttableFrontTile(t *testing.T) {
	for _, tc := range []struct {
		tile uint8
		want bool
	}{
		{cutTreeTile, true},
		{gymCutTreeTile, true},
		{0x52, false}, // grass is cuttable by the game but never a route gate
		{0x00, false},
	} {
		if got := cuttableFrontTile(tc.tile); got != tc.want {
			t.Errorf("cuttableFrontTile(%#02x) = %v, want %v", tc.tile, got, tc.want)
		}
	}
}

func TestMonKnowsCut(t *testing.T) {
	mon := state.Mon{Moves: [4]uint8{0x21, cutMove, 0x37, 0}}
	if !monKnowsMove(mon, cutMove) {
		t.Fatal("monKnowsMove did not find Cut")
	}
	if monKnowsMove(mon, 0x55) {
		t.Fatal("monKnowsMove reported a move that is not present")
	}
}

func TestVermilionCityOffersSurgeChallenge(t *testing.T) {
	city, ok := GymAt(vermilionCity)
	if !ok {
		t.Fatal("Vermilion City has no gym challenge")
	}
	inside, ok := GymAt(vermilionGymMap)
	if !ok {
		t.Fatal("Vermilion Gym has no gym challenge")
	}
	if city.Leader != "LT. SURGE" || city.Badge != state.BadgeThunder {
		t.Fatalf("city challenge = %+v, want Lt. Surge / Thunder Badge", city)
	}
	if city != inside {
		t.Fatalf("city and interior challenges differ: city=%+v inside=%+v", city, inside)
	}
}
