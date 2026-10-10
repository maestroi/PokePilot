package agent

import (
	"testing"

	gsdata "github.com/maestroi/pokepilot/gs/data"
)

func TestGSCoastalHMAndGymCoordinatesMatchDecomp(t *testing.T) {
	// From pokegold/maps/OlivineCafe.asm, CianwoodGym.asm,
	// CianwoodCity.asm and OlivineGym.asm.
	if gsStrengthSailorX != 4 || gsStrengthSailorY != 3 ||
		gsStrengthSailorStandX != 4 || gsStrengthSailorStandY != 4 {
		t.Fatalf("Strength sailor = (%d,%d), staging = (%d,%d)",
			gsStrengthSailorX, gsStrengthSailorY,
			gsStrengthSailorStandX, gsStrengthSailorStandY)
	}
	if gsChuckX != 4 || gsChuckY != 1 || gsChuckStandX != 4 || gsChuckStandY != 2 {
		t.Fatalf("Chuck = (%d,%d), staging = (%d,%d)",
			gsChuckX, gsChuckY, gsChuckStandX, gsChuckStandY)
	}
	if gsChucksWifeSlot != 10 || gsChucksWifeStandX != 10 || gsChucksWifeStandY != 45 {
		t.Fatalf("wife live object slot/stand = %d (%d,%d)",
			gsChucksWifeSlot, gsChucksWifeStandX, gsChucksWifeStandY)
	}
	if gsJasmineGymX != 5 || gsJasmineGymY != 3 ||
		gsJasmineGymStandX != 5 || gsJasmineGymStandY != 4 {
		t.Fatalf("Jasmine = (%d,%d), staging = (%d,%d)",
			gsJasmineGymX, gsJasmineGymY, gsJasmineGymStandX, gsJasmineGymStandY)
	}
}

func TestGSCoastalGymsAreOwnedWithLocalHealing(t *testing.T) {
	for _, tc := range []struct {
		city, gym, center string
	}{
		{"OLIVINE_CITY", "OLIVINE_GYM", "OLIVINE_POKECENTER_1F"},
		{"CIANWOOD_CITY", "CIANWOOD_GYM", "CIANWOOD_POKECENTER_1F"},
	} {
		city, err := gsOpeningMapID(tc.city)
		if err != nil {
			t.Fatal(err)
		}
		gym, err := gsOpeningMapID(tc.gym)
		if err != nil {
			t.Fatal(err)
		}
		center, err := gsOpeningMapID(tc.center)
		if err != nil {
			t.Fatal(err)
		}
		if !gsSecondBadgeOwnedMap(gym) || !gsSecondBadgeOwnedMap(center) {
			t.Fatalf("coastal gym ownership %s=%v center %s=%v",
				tc.gym, gsSecondBadgeOwnedMap(gym), tc.center, gsSecondBadgeOwnedMap(center))
		}
		local, err := gsOwnedPokecenterID(city)
		if err != nil || local != center {
			t.Fatalf("%s center = %#04x (%v), want %#04x", tc.city, local, err, center)
		}
		if info, ok := gsdata.Map(gym); !ok || info.Name != tc.gym {
			t.Fatalf("native catalog lacks %s: %+v", tc.gym, info)
		}
	}
	cafe, err := gsOpeningMapID("OLIVINE_CAFE")
	if err != nil || !gsSecondBadgeOwnedMap(cafe) {
		t.Fatalf("Olivine Cafe is outside owned corridor: id %#04x err=%v", cafe, err)
	}
}
