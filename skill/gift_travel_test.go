package skill

import "testing"

func TestEeveeGiftDestinationUsesReachableLateralStand(t *testing.T) {
	got := eeveeGiftDestination()
	if got.Map != celadonMansionRoofHouseMap || got.X != 3 || got.Y != 3 {
		t.Fatalf("Eevee gift destination = %+v, want map %#02x at (3,3)", got, celadonMansionRoofHouseMap)
	}

	place, ok := Place("celadon mansion eevee")
	if !ok {
		t.Fatal("celadon mansion eevee interaction place missing")
	}
	if place != got {
		t.Fatalf("Eevee interaction place = %+v, executor destination = %+v", place, got)
	}
}

func TestFightingDojoGiftDestinationsUsePrizeStandTiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		x    uint8
		y    uint8
	}{
		{name: "fighting dojo hitmonlee", x: hitmonleeGiftX, y: hitmonleeGiftY},
		{name: "fighting dojo hitmonchan", x: hitmonchanGiftX, y: hitmonchanGiftY},
	} {
		want := fightingDojoGiftDestination(tc.x, tc.y)
		if want.Map != fightingDojoMap || want.X != tc.x || want.Y != tc.y+1 {
			t.Fatalf("%s executor destination = %+v, want map %#02x at (%d,%d)", tc.name, want, fightingDojoMap, tc.x, tc.y+1)
		}
		place, ok := Place(tc.name)
		if !ok {
			t.Fatalf("%s interaction place missing", tc.name)
		}
		if place != want {
			t.Fatalf("%s interaction place = %+v, executor destination = %+v", tc.name, place, want)
		}
	}
}
