package agent

import (
	"os"
	"strings"
	"testing"

	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

// TestGSPokecenterCounterMatchesROMGrid guards triage:2bb48288867cac27:
// Indigo Plateau healed from the generic (3,3), which is a wall in its lobby.
func TestGSPokecenterCounterMatchesROMGrid(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("set POKEMON_GOLD_ROM to check center tiles against the live grid")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := gsrom.NewWorldProvider(romData)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"INDIGO_PLATEAU_POKECENTER_1F", "BLACKTHORN_POKECENTER_1F"}, gsOwnedPokecenterNames...) {
		center, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatal(err)
		}
		grid, err := provider.Grid(center, nil, worldmodel.TraversalMode(0))
		if err != nil {
			t.Fatal(err)
		}
		walkable := func(x, y uint8) bool { return grid.Walkable[int(y)*grid.Width+int(x)] }
		cx, cy := gsPokecenterCounter(center)
		if walkable(cx, cy) || !walkable(cx, cy+1) {
			t.Errorf("%s: counter (%d,%d) walkable=%v, stand (%d,%d) walkable=%v",
				name, cx, cy, walkable(cx, cy), cx, cy+1, walkable(cx, cy+1))
		}
	}
}

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
