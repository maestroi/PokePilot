package agent

import (
	"testing"

	gsdata "github.com/maestroi/pokepilot/gs/data"
)

func TestGSOlivineCianwoodRetailInteractionCoordinates(t *testing.T) {
	if gsJasmineLighthouseX != 8 || gsJasmineLighthouseY != 8 ||
		gsJasmineLighthouseStandX != 8 || gsJasmineLighthouseStandY != 9 {
		t.Fatalf("Lighthouse Jasmine (%d,%d), stand (%d,%d), expected (8,8), (8,9)",
			gsJasmineLighthouseX, gsJasmineLighthouseY,
			gsJasmineLighthouseStandX, gsJasmineLighthouseStandY)
	}
	if gsCianwoodPharmacistX != 2 || gsCianwoodPharmacistY != 3 ||
		gsCianwoodPharmacyStandX != 2 || gsCianwoodPharmacyStandY != 4 {
		t.Fatalf("Cianwood pharmacist (%d,%d), stand (%d,%d), expected (2,3), (2,4)",
			gsCianwoodPharmacistX, gsCianwoodPharmacistY,
			gsCianwoodPharmacyStandX, gsCianwoodPharmacyStandY)
	}
}

func TestGSOlivineCianwoodOwnedNavigationCorridor(t *testing.T) {
	for _, name := range []string{
		"ROUTE_38", "ROUTE_39", "OLIVINE_CITY", "OLIVINE_POKECENTER_1F",
		"OLIVINE_LIGHTHOUSE_1F", "OLIVINE_LIGHTHOUSE_2F",
		"OLIVINE_LIGHTHOUSE_3F", "OLIVINE_LIGHTHOUSE_4F",
		"OLIVINE_LIGHTHOUSE_5F", "OLIVINE_LIGHTHOUSE_6F",
		"ROUTE_40", "ROUTE_41", "CIANWOOD_CITY",
		"CIANWOOD_POKECENTER_1F", "CIANWOOD_PHARMACY",
	} {
		mapID, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatalf("retail map %s missing: %v", name, err)
		}
		if !gsSecondBadgeOwnedMap(mapID) {
			t.Fatalf("script interruption on %s (%#04x) is not owned", name, mapID)
		}
	}
	for _, name := range []string{"OLIVINE_POKECENTER_1F", "CIANWOOD_POKECENTER_1F"} {
		source, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatal(err)
		}
		local, err := gsOwnedPokecenterID(source)
		if err != nil || local != source {
			t.Fatalf("%s must have local center; got %#04x err %v", name, local, err)
		}
		if info, ok := gsdata.MapByNative(source); !ok || info.Name != name {
			t.Fatalf("native catalog lost %s: %+v", name, info)
		}
	}
}
