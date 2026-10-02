package agent

import "testing"

func TestGSEcruteakSliceCoordinatesMatchPinnedDecomp(t *testing.T) {
	if gsFlowerShopTeacherX != 2 || gsFlowerShopTeacherY != 4 ||
		gsFlowerShopStandX != 3 || gsFlowerShopStandY != 4 {
		t.Fatalf("Flower Shop interaction = teacher(%d,%d) stand(%d,%d), want teacher(2,4) stand(3,4)",
			gsFlowerShopTeacherX, gsFlowerShopTeacherY, gsFlowerShopStandX, gsFlowerShopStandY)
	}
	if gsSudowoodoX != 35 || gsSudowoodoY != 9 ||
		gsSudowoodoStandX != 35 || gsSudowoodoStandY != 10 {
		t.Fatalf("Sudowoodo interaction = tree(%d,%d) stand(%d,%d), want tree(35,9) stand(35,10)",
			gsSudowoodoX, gsSudowoodoY, gsSudowoodoStandX, gsSudowoodoStandY)
	}
	if gsBurnedTowerBeastsX != 9 || gsBurnedTowerBeastsY != 5 {
		t.Fatalf("Burned Tower beasts trigger = (%d,%d), want (9,5)", gsBurnedTowerBeastsX, gsBurnedTowerBeastsY)
	}
	if gsMortyX != 5 || gsMortyY != 1 || gsMortyStandX != 5 || gsMortyStandY != 2 {
		t.Fatalf("Morty interaction = leader(%d,%d) stand(%d,%d), want leader(5,1) stand(5,2)",
			gsMortyX, gsMortyY, gsMortyStandX, gsMortyStandY)
	}
}

func TestGSEcruteakSliceMapsAreOwned(t *testing.T) {
	for _, name := range []string{
		"GOLDENROD_FLOWER_SHOP",
		"ROUTE_35",
		"ROUTE_36",
		"ROUTE_37",
		"ECRUTEAK_CITY",
		"ECRUTEAK_POKECENTER_1F",
		"BURNED_TOWER_1F",
		"BURNED_TOWER_B1F",
		"ECRUTEAK_GYM",
	} {
		id, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("%s (%#04x) is outside owned Gen2 corridor", name, id)
		}
	}
}

func TestGSEcruteakGymRecoveryUsesLocalCenter(t *testing.T) {
	gym, err := gsOpeningMapID("ECRUTEAK_GYM")
	if err != nil {
		t.Fatal(err)
	}
	center, err := gsOpeningMapID("ECRUTEAK_POKECENTER_1F")
	if err != nil {
		t.Fatal(err)
	}
	got, err := gsOwnedPokecenterID(gym)
	if err != nil {
		t.Fatal(err)
	}
	if got != center {
		t.Fatalf("Ecruteak recovery center = %#04x, want %#04x", got, center)
	}
}
