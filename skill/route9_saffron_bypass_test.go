package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestRoute9RoutesToCeladonWithoutSaffron reproduces run-1948e1rnco3sp1y9bbhdwp7eov:
// three badges (Boulder/Cascade/Thunder), a Cut-capable party member, and the
// S.S. Ticket + HM01 already in the bag, standing in Cerulean City with the
// Saffron guard drink not yet given. PostSurgeCeladonProgression's own
// Rainbow Badge leg depends on the router finding the Cerulean -> Route 9 ->
// Rock Tunnel -> Lavender -> Route 8 -> Underground Path -> Route 7 ->
// Celadon detour; the farm run instead reported "world: no route" and fell
// back to a Saffron path that was still closed. Requires POKEMON_RED_ROM.
func TestRoute9RoutesToCeladonWithoutSaffron(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	var mem state.Mem
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade | 1<<state.BadgeThunder
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 6 // Charmeleon: Cut-compatible
	copy(mem.Slice(base+sym.MonMoves, 4), []byte{cutMove, 0, 0, 0})
	mem[sym.NumBagItems] = 2
	mem[sym.BagItems] = ssTicketItem
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = hm01Item
	mem[sym.BagItems+3] = 1
	mem[sym.BagItems+4] = 0xff

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, &mem, redWram())
	if !prereqs.Capabilities.Has(capCanCut) {
		t.Fatalf("capabilities did not include %q: %v", capCanCut, prereqs.Capabilities)
	}

	const ceruleanMap, celadonPCMap = 0x03, 0x85
	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, ceruleanMap, celadonPCMap, 5, 18, 3, 3, nil, prereqs,
	)
	if err != nil {
		t.Fatalf("no route from Cerulean to Celadon Pokemon Center with can_cut and no Saffron access: %v", err)
	}
	sawSaffron := false
	for _, step := range route {
		if step.Edge.From == semanticSaffronCityMap || step.Edge.To == semanticSaffronCityMap {
			sawSaffron = true
		}
	}
	if sawSaffron {
		t.Fatalf("route to Celadon crossed Saffron despite the guard drink not being given: %+v", route)
	}
}
