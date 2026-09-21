package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestCeladonRoutesToCinnabarWithSurf pins farm triage ff3ad54bb21d3a2d
// (run-3uwfb4to132uw2i3ohvjn1kuge): progress secret_key_owned died with
// world.ErrNoRoute while forcing a Pallet waypoint from Celadon (41,10). Diglett's
// Cave cannot walk to Viridian/Pallet, but with Surf the southern-sea approach
// from Fuchsia must be plannable so AcquireCinnabarSecretKey can Travel straight
// to Cinnabar.
func TestCeladonRoutesToCinnabarWithSurf(t *testing.T) {
	romData := badgeFourROM(t)

	mem := fieldTestMem(FieldSurf, true, true, true)
	// Unlock Snorlax / Saffron / early gates the southern approach may touch.
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade |
		1<<state.BadgeThunder | 1<<state.BadgeRainbow | 1<<state.BadgeSoul |
		1<<state.BadgeMarsh
	mem[sym.NumBagItems] = 2
	mem[sym.BagItems] = hm03SurfItem
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = 0x49 // Poke Flute
	mem[sym.BagItems+3] = 1
	mem[sym.BagItems+4] = 0xff
	mem[sym.StatusFlags1] = 1 << 6 // BIT_GAVE_SAFFRON_GUARDS_DRINK

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, mem)
	if !prereqs.Capabilities.Has(capCanSurf) {
		t.Fatalf("capabilities missing %q: %v", capCanSurf, prereqs.Capabilities)
	}

	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, celadonCityMap, cinnabarIslandMap, 41, 10, 11, 12, nil, prereqs,
	)
	if err != nil {
		t.Fatalf("Celadon (41,10) -> Cinnabar with Surf: %v", err)
	}
	if len(route) == 0 {
		t.Fatal("empty route")
	}
	sawSurf := false
	for _, step := range route {
		if step.Transition != nil &&
			(step.Transition.ID == "red:southern_sea_surf" || step.Transition.ID == "red:route21_surf") {
			sawSurf = true
			break
		}
	}
	if !sawSurf {
		t.Fatalf("route to Cinnabar had no Surf transition: %+v", route)
	}

	// Land-only planning from the same tile must still fail toward Pallet: the
	// Diglett pocket remains Cut-sealed, so the old Pallet waypoint stays a
	// wrong forced path even after Surf shores are routable.
	var land state.Mem
	land = *mem
	landCaps := redRoutePrerequisites(g, romData, &land)
	delete(landCaps.Capabilities, capCanSurf)
	_, landErr := world.FindRoutePlanAtDestinationWithCapabilities(
		g, celadonCityMap, semanticPalletTownMap, 41, 10, 5, 6, nil, landCaps,
	)
	if !errors.Is(landErr, world.ErrNoRoute) {
		t.Fatalf("without Surf, Celadon->Pallet error = %v, want ErrNoRoute", landErr)
	}
}
