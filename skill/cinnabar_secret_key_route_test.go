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
