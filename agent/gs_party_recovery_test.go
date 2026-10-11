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

func TestGSOwnedPokecenterIsNearestNotFirstInGroup(t *testing.T) {
	// Route 37 shares Violet's map group, but Ecruteak is the adjacent city.
	route37, err := gsOpeningMapID("ROUTE_37")
	if err != nil {
		t.Fatal(err)
	}
	got, err := gsOwnedPokecenterID(route37)
	if err != nil {
		t.Fatal(err)
	}
	if info, ok := gsdata.Map(got); !ok || info.Name != "ECRUTEAK_POKECENTER_1F" {
		t.Fatalf("Route 37 center = %+v, want ECRUTEAK_POKECENTER_1F", info)
	}
}

func TestGSCoastalRecoveryPrefersReachableNearbyCenters(t *testing.T) {
	for _, tc := range []struct {
		from string
		want string
	}{
		{"OLIVINE_LIGHTHOUSE_6F", "OLIVINE_POKECENTER_1F"},
		{"OLIVINE_GYM", "OLIVINE_POKECENTER_1F"},
		{"ROUTE_40", "OLIVINE_POKECENTER_1F"},
		{"CIANWOOD_GYM", "CIANWOOD_POKECENTER_1F"},
		{"CIANWOOD_PHARMACY", "CIANWOOD_POKECENTER_1F"},
	} {
		t.Run(tc.from, func(t *testing.T) {
			from, err := gsOpeningMapID(tc.from)
			if err != nil {
				t.Fatal(err)
			}
			want, err := gsOpeningMapID(tc.want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := gsOwnedPokecenterID(from)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("center for %s = %#04x; want %s (%#04x)", tc.from, got, tc.want, want)
			}
		})
	}
}
