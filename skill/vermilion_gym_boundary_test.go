package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestVermilionGymNotOfferedAsBoundaryTowardCinnabar pins the regression from
// run-14itq6xawle0136xfk2l4gkqfq (GitHub #1525): AcquireCinnabarSecretKey's
// GoTo(Vermilion City -> Cinnabar Island) repeatedly walked into the
// Cut-gated Vermilion Gym door and back out, until the navigation guard
// fired on the exact-state repeat (trace: 05(19,0) -> 5c(4,17) -> 05(12,20)
// -> 5c(4,17)).
//
// The gym's Cut-gated warp (route_semantics.go's "red:vermilion_gym_cut")
// sets neither Gate, PortBypass, nor PivotOnly, so it fell into
// classify's/classifyPrivileges' default case, which grants relaxLanding
// ("this is a live-topology boundary, stop and replan here") to every such
// transition regardless of edge kind. findRoute (world/route.go) then
// offered it back as the first-discovered "safe prefix" boundary toward
// Cinnabar even though the gym is a fully known, ROM-static, one-exit dead
// end — nothing about it needs live replay the way a Surf shore does.
//
// This is the same shape as Celadon Gym's identically modeled Cut door
// (route_gate_audit.go: "Model the door as the same bidirectional pivot
// used for Vermilion Gym"), so the fix is generic: only EdgeConnection
// crossings (whose far-side geometry really is unknown until observed live)
// get boundary treatment; a gated EdgeWarp's destination is already fully
// known from ROM and must not stop the search.
func TestVermilionGymNotOfferedAsBoundaryTowardCinnabar(t *testing.T) {
	romData := badgeFourROM(t)

	mem := new(state.Mem)
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = 0x0F   // Cut
	mem[sym.PartyMon1+sym.MonMoves+1] = 0x39 // Surf
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade |
		1<<state.BadgeThunder | 1<<state.BadgeRainbow | 1<<state.BadgeSoul | 1<<state.BadgeMarsh
	mem[sym.NumBagItems] = 2
	mem[sym.BagItems] = hm01Item
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = hm03SurfItem
	mem[sym.BagItems+3] = 1
	mem[sym.BagItems+4] = 0xff

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, mem)
	if !prereqs.Capabilities.Has(capCanCut) {
		t.Fatalf("capabilities missing %q: %v", capCanCut, prereqs.Capabilities)
	}
	if !prereqs.Capabilities.Has(capCanSurf) {
		t.Fatalf("capabilities missing %q: %v", capCanSurf, prereqs.Capabilities)
	}

	// Vermilion City's dock tile (12,20): the same tile the failing run's
	// navigation guard recorded standing on just before it walked into the
	// gym a second time.
	route, _ := world.FindRoutePlanAtDestinationWithCapabilities(
		g, vermilionCity, cinnabarIslandMap, 12, 20, 11, 12, nil, prereqs,
	)
	for _, step := range route {
		if step.Edge.To == vermilionGymMap {
			t.Fatalf("route toward Cinnabar offered a leg into Vermilion Gym (%#04x), a proven one-exit dead end: %+v", vermilionGymMap, route)
		}
	}
}
