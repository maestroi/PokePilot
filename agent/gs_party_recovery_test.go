package agent

import (
	"strings"
	"testing"

	gsdata "github.com/maestroi/pokepilot/gs/data"
)

func TestGSPokecenterStandFacesCounter(t *testing.T) {
	if gsPokecenterStandX != gsPokecenterCounterX || int(gsPokecenterStandY) != int(gsPokecenterCounterY)+1 {
		t.Fatalf("stand (%d,%d) is not the tile south of the counter (%d,%d)",
			gsPokecenterStandX, gsPokecenterStandY, gsPokecenterCounterX, gsPokecenterCounterY)
	}
	if gsPokecenterNurseX != gsPokecenterCounterX || int(gsPokecenterNurseY)+1 != int(gsPokecenterCounterY) {
		t.Fatalf("nurse (%d,%d) is not the tile north of the counter (%d,%d)",
			gsPokecenterNurseX, gsPokecenterNurseY, gsPokecenterCounterX, gsPokecenterCounterY)
	}
}

func TestGSOwnedPokecenterPrefersSameMapGroup(t *testing.T) {
	azaleaGym, err := gsOpeningMapID("AZALEA_GYM")
	if err != nil {
		t.Fatal(err)
	}
	got, err := gsOwnedPokecenterID(azaleaGym)
	if err != nil {
		t.Fatal(err)
	}
	info, ok := gsdata.Map(got)
	if !ok || info.Name != "AZALEA_POKECENTER_1F" {
		t.Fatalf("Azalea gym local center = %+v (%s), want AZALEA_POKECENTER_1F", info, err)
	}

	violet, err := gsOpeningMapID("VIOLET_CITY")
	if err != nil {
		t.Fatal(err)
	}
	got, err = gsOwnedPokecenterID(violet)
	if err != nil {
		t.Fatal(err)
	}
	info, ok = gsdata.Map(got)
	if !ok || !strings.Contains(info.Name, "VIOLET") || !strings.Contains(info.Name, "POKECENTER") {
		t.Fatalf("Violet local center = %+v", info)
	}
}
