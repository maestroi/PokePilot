package agent

import "testing"

func TestGSWhitneyScriptCoordinatesMatchPinnedGoldenrodGym(t *testing.T) {
	if gsWhitneyX != 8 || gsWhitneyY != 3 {
		t.Fatalf("Whitney = (%d,%d), want (8,3)", gsWhitneyX, gsWhitneyY)
	}
	if gsWhitneyStandX != 8 || gsWhitneyStandY != 4 {
		t.Fatalf("Whitney stand = (%d,%d), want (8,4)", gsWhitneyStandX, gsWhitneyStandY)
	}
	if gsWhitneyCryX != 8 || gsWhitneyCryY != 5 {
		t.Fatalf("Whitney crying coord event = (%d,%d), want (8,5)", gsWhitneyCryX, gsWhitneyCryY)
	}
}

func TestGSWhitneyCorridorIsOwnedAndUsesGoldenrodCenter(t *testing.T) {
	city, err := gsOpeningMapID("GOLDENROD_CITY")
	if err != nil {
		t.Fatal(err)
	}
	gym, err := gsOpeningMapID("GOLDENROD_GYM")
	if err != nil {
		t.Fatal(err)
	}
	center, err := gsOpeningMapID("GOLDENROD_POKECENTER_1F")
	if err != nil {
		t.Fatal(err)
	}
	if !gsSecondBadgeOwnedMap(gym) || !gsSecondBadgeOwnedMap(center) {
		t.Fatalf("Goldenrod runtime ownership: gym=%v center=%v", gsSecondBadgeOwnedMap(gym), gsSecondBadgeOwnedMap(center))
	}
	got, err := gsOwnedPokecenterID(city)
	if err != nil {
		t.Fatal(err)
	}
	if got != center {
		t.Fatalf("Goldenrod recovery center = %#04x, want %#04x", got, center)
	}
}
