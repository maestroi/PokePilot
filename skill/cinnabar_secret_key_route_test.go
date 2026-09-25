package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestSecretKeyPalletRouteUsesRoute21 pins #1595. Secret Key deliberately
// stages at Pallet before travelling to Cinnabar: Route 20 is split by
// Seafoam Islands and must not become an accidental prerequisite of this
// milestone merely because both outside seams require Surf.
func TestSecretKeyPalletRouteUsesRoute21(t *testing.T) {
	romData := badgeFourROM(t)

	mem := fieldTestMem(FieldSurf, true, true, true)
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade |
		1<<state.BadgeThunder | 1<<state.BadgeRainbow | 1<<state.BadgeSoul |
		1<<state.BadgeMarsh
	mem[sym.NumBagItems] = 2
	mem[sym.BagItems] = hm03SurfItem
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = 0x49 // Poke Flute
	mem[sym.BagItems+3] = 1
	mem[sym.BagItems+4] = 0xff

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, mem)
	if !prereqs.Capabilities.Has(capCanSurf) {
		t.Fatalf("capabilities missing %q: %v", capCanSurf, prereqs.Capabilities)
	}

	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, semanticPalletTownMap, cinnabarIslandMap, 5, 6, 11, 12, nil, prereqs,
	)
	if err != nil {
		t.Fatalf("Pallet -> Cinnabar with Surf: %v", err)
	}
	if len(route) == 0 {
		t.Fatal("empty Pallet -> Cinnabar route")
	}

	sawRoute21Surf := false
	for _, step := range route {
		if step.Edge.From == route20Map || step.Edge.To == route20Map {
			t.Fatalf("Secret Key Route 21 corridor detoured through Route 20/Seafoam: %+v", route)
		}
		if step.Transition == nil {
			continue
		}
		if step.Transition.ID == "red:southern_sea_surf" {
			t.Fatalf("Secret Key Route 21 corridor selected southern-sea Surf: %+v", route)
		}
		if step.Transition.ID == "red:route21_surf" {
			sawRoute21Surf = true
		}
	}
	if !sawRoute21Surf {
		t.Fatalf("Pallet -> Cinnabar route did not use Route 21 Surf: %+v", route)
	}
}

func TestSecretKeyRoute20ResumeComponentsExitAwayFromSeafoam(t *testing.T) {
	romData := badgeFourROM(t)

	mem := fieldTestMem(FieldSurf, true, true, true)
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade |
		1<<state.BadgeThunder | 1<<state.BadgeRainbow | 1<<state.BadgeSoul |
		1<<state.BadgeMarsh
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, mem)
	if !prereqs.Capabilities.Has(capCanSurf) {
		t.Fatalf("capabilities missing %q: %v", capCanSurf, prereqs.Capabilities)
	}

	cinnabar := Destination{Map: cinnabarIslandMap, X: 11, Y: 12}
	west, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route20Map, cinnabar.Map, 0, 10, int(cinnabar.X), int(cinnabar.Y), nil, prereqs,
	)
	if err != nil {
		t.Fatalf("west Route 20 -> Cinnabar: %v", err)
	}
	if len(west) == 0 || west[0].Edge.To != cinnabarIslandMap {
		t.Fatalf("west Route 20 did not exit directly to Cinnabar: %+v", west)
	}
	for _, step := range west {
		if step.Edge.To == seafoam1FMap {
			t.Fatalf("west Route 20 resume entered Seafoam: %+v", west)
		}
	}

	fuchsia, ok := Place("fuchsia city")
	if !ok {
		t.Fatal("fuchsia city place missing")
	}
	east, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route20Map, fuchsia.Map, 99, 10, int(fuchsia.X), int(fuchsia.Y), nil, prereqs,
	)
	if err != nil {
		t.Fatalf("east Route 20 -> Fuchsia: %v", err)
	}
	if len(east) == 0 || east[0].Edge.To != route19Map {
		t.Fatalf("east Route 20 did not exit through Route 19: %+v", east)
	}
	for _, step := range east {
		if step.Edge.To == seafoam1FMap {
			t.Fatalf("east Route 20 resume entered Seafoam: %+v", east)
		}
	}
}
