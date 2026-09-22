package skill

import "testing"

func TestEeveeGiftDestinationUsesLiveInteractionApproach(t *testing.T) {
	got := eeveeGiftDestination()
	if got.Map != celadonMansionRoofHouseMap || got.X != eeveeGiftX || got.Y != eeveeGiftY || got.Kind != DestinationInteraction {
		t.Fatalf("Eevee gift destination = %+v, want interaction map %#02x target (%d,%d)",
			got, celadonMansionRoofHouseMap, eeveeGiftX, eeveeGiftY)
	}

	place, ok := Place("celadon mansion eevee")
	if !ok {
		t.Fatal("celadon mansion eevee interaction place missing")
	}
	if place != got {
		t.Fatalf("Eevee interaction place = %+v, executor destination = %+v", place, got)
	}

	// Cross-map planning must be free to choose whichever side of the ball is
	// actually connected when Red arrives. The old exact destination pinned the
	// route to one guessed stand tile and reintroduced #425 as #1577.
	targets := destinationRouteTargets(got)
	want := map[destinationRouteTarget]bool{
		{X: int(eeveeGiftX) - 1, Y: int(eeveeGiftY)}: true,
		{X: int(eeveeGiftX) + 1, Y: int(eeveeGiftY)}: true,
		{X: int(eeveeGiftX), Y: int(eeveeGiftY) - 1}: true,
		{X: int(eeveeGiftX), Y: int(eeveeGiftY) + 1}: true,
	}
	for _, target := range targets {
		delete(want, target)
	}
	if len(want) != 0 {
		t.Fatalf("Eevee interaction route targets missing adjacent approaches: %v (all=%v)", want, targets)
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
