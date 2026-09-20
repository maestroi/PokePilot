package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestRoute9CutToLavenderUsesRockTunnel pins farm run-2ccw7p3rpnvu4129l1dkhc7ayh:
// with can_cut, PortBypass on Route 9 -> Route 10 used to also skip phantom
// east-seam bands whose exitComps were empty. Those hops landed on Route 10
// with a nil component set, so the planner invented the south Lavender
// connection from the north side and GoTo bounced Rock Tunnel until
// navigation_stalled. The real Cut bridge must keep Route 10's north landing
// and still route through the cave.
func TestRoute9CutToLavenderUsesRockTunnel(t *testing.T) {
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
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = hm01Item
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = 0xff

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if !prereqs.Capabilities.Has(capCanCut) {
		t.Fatalf("capabilities did not include %q: %v", capCanCut, prereqs.Capabilities)
	}

	lavender, ok := Place("lavender town")
	if !ok {
		t.Fatal("lavender town place missing")
	}
	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, semanticRoute9Map, lavender.Map, 25, 8, int(lavender.X), int(lavender.Y), nil, prereqs,
	)
	if err != nil {
		t.Fatalf("Route 9 (25,8) -> Lavender with can_cut: %v", err)
	}
	if len(route) < 3 {
		t.Fatalf("route has %d leg(s): %+v; want Rock Tunnel, not Route10 south", len(route), route)
	}
	usedRockTunnel := false
	for _, step := range route {
		e := step.Edge
		if e.From == 0x52 || e.To == 0x52 || e.From == 0xE8 || e.To == 0xE8 {
			usedRockTunnel = true
		}
		if e.From == route10Map && e.To == lavender.Map && e.Kind == world.EdgeConnection {
			// A direct south hop is only legal after exiting Rock Tunnel onto
			// Route 10's south component. Seeing it as leg 2 (or earlier) means
			// the planner invented it from the north landing.
			if !usedRockTunnel {
				t.Fatalf("planned Route 10 south before Rock Tunnel: %+v", route)
			}
		}
	}
	if !usedRockTunnel {
		t.Fatalf("Route 9 -> Lavender with can_cut skipped Rock Tunnel: %+v", route)
	}
}
