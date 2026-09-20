package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestRoute9CutIsPivotOnlyWhenLeavingRoute9(t *testing.T) {
	cases := []struct {
		name           string
		edge           world.Edge
		wantPivot      bool
		wantPortBypass bool
	}{
		{
			name:      "Route 9 -> Cerulean",
			edge:      world.Edge{Kind: world.EdgeConnection, From: semanticRoute9Map, To: semanticCeruleanCityMap},
			wantPivot: true,
		},
		{
			name:           "Route 9 -> Route 10",
			edge:           world.Edge{Kind: world.EdgeConnection, From: semanticRoute9Map, To: route10Map},
			wantPivot:      true,
			wantPortBypass: true,
		},
		{
			name:      "Cerulean -> Route 9 stays ordinary",
			edge:      world.Edge{Kind: world.EdgeConnection, From: semanticCeruleanCityMap, To: semanticRoute9Map},
			wantPivot: false,
		},
		{
			name:      "Route 10 -> Route 9 stays ordinary",
			edge:      world.Edge{Kind: world.EdgeConnection, From: route10Map, To: semanticRoute9Map},
			wantPivot: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transition, ok := redRouteTransitionForEdge(tc.edge)
			if tc.wantPivot != ok {
				t.Fatalf("ok=%v wantPivot=%v transition=%+v", ok, tc.wantPivot, transition)
			}
			if !tc.wantPivot {
				return
			}
			if transition.ID != "red:route9_cut" || transition.Gate || !transition.PivotOnly {
				t.Fatalf("transition=%+v, want PivotOnly red:route9_cut", transition)
			}
			if transition.PortBypass != tc.wantPortBypass {
				t.Fatalf("PortBypass=%v, want %v", transition.PortBypass, tc.wantPortBypass)
			}
			if len(transition.Requires) != 1 || transition.Requires[0] != capCanCut {
				t.Fatalf("requirements=%v, want [%s]", transition.Requires, capCanCut)
			}
		})
	}
}

// TestRoute9WestRoutesToLavenderWithoutSaffron pins run-os1jmuuqpc1033zjhq2at3qz4:
// stranded west of Route 9's Cut tree with can_cut, GoTo must reach Lavender
// through Rock Tunnel instead of reporting can_enter_saffron.
func TestRoute9WestRoutesToLavenderWithoutSaffron(t *testing.T) {
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
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if !prereqs.Capabilities.Has(capCanCut) {
		t.Fatalf("capabilities did not include %q: %v", capCanCut, prereqs.Capabilities)
	}
	if prereqs.Capabilities.Has(capCanEnterSaffron) {
		t.Fatal("test setup must not include can_enter_saffron")
	}

	lavender, ok := Place("lavender town")
	if !ok {
		t.Fatal("lavender town place missing")
	}
	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, semanticRoute9Map, lavender.Map, 0, 8, int(lavender.X), int(lavender.Y), nil, prereqs,
	)
	if err != nil {
		t.Fatalf("Route 9 (0,8) -> Lavender with can_cut and no Saffron: %v", err)
	}
	sawSaffron := false
	for _, step := range route {
		if step.Edge.From == semanticSaffronCityMap || step.Edge.To == semanticSaffronCityMap {
			sawSaffron = true
		}
	}
	if sawSaffron {
		t.Fatalf("route crossed Saffron despite missing drink: %+v", route)
	}
	// A Route 9 -> Route 10 Cut hop is enough to prove the west-side checkpoint
	// is no longer diagnosed as can_enter_saffron. Some east-edge bands lack
	// static entry components, so the shortest plan may temporarily look like a
	// direct Route 10 south seam; live Cut + replan still owns Rock Tunnel.
	sawRoute10 := false
	for _, step := range route {
		if step.Edge.From == route10Map || step.Edge.To == route10Map {
			sawRoute10 = true
			break
		}
	}
	if !sawRoute10 {
		t.Fatalf("route did not leave via Route 10: %+v", route)
	}
}

// TestRockTunnelNorthRoutesToLavenderThroughTunnel pins farm triage
// bd4bb51aa6b8d736 (run-3fl35ipa0wah83axgm9bz5mqc9): standing in Rock Tunnel
// 1F's north pocket with can_cut, GoTo must traverse B1F to Route 10 south.
// A PortBypass+PivotOnly route9_cut annotation on phantom Route 9 connection
// bands used to invent Route 10 north -> Route 9 Cut -> Route 10 south, which
// bounced until navigation_stalled.
func TestRockTunnelNorthRoutesToLavenderThroughTunnel(t *testing.T) {
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
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if !prereqs.Capabilities.Has(capCanCut) {
		t.Fatalf("capabilities did not include %q: %v", capCanCut, prereqs.Capabilities)
	}

	lavender, ok := Place("lavender town")
	if !ok {
		t.Fatal("lavender town place missing")
	}
	const rockTunnel1F = uint8(0x52)
	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, rockTunnel1F, lavender.Map, 15, 4, int(lavender.X), int(lavender.Y), nil, prereqs,
	)
	if err != nil {
		t.Fatalf("Rock Tunnel 1F (15,4) -> Lavender with can_cut: %v", err)
	}
	if len(route) == 0 {
		t.Fatal("empty route")
	}
	if route[0].Edge.To == route10Map {
		t.Fatalf("first hop exited north to Route 10 instead of B1F: %+v", route)
	}
	sawB1F := false
	sawRoute9Cut := false
	for _, step := range route {
		if step.Edge.To == 0xe8 || step.Edge.From == 0xe8 {
			sawB1F = true
		}
		if step.Transition != nil && step.Transition.ID == "red:route9_cut" {
			sawRoute9Cut = true
		}
	}
	if !sawB1F {
		t.Fatalf("route did not traverse Rock Tunnel B1F: %+v", route)
	}
	if sawRoute9Cut {
		t.Fatalf("route invented Route 9 Cut bypass instead of tunnel: %+v", route)
	}
}

// TestGoToRoute4FromCeruleanEastDoesNotBounceRoute9 is farm #1261
// (run-27dtzi7qnqt962i4ecootzj8tg): GoTo Route 4 (10,10) from Cerulean's
// east seam with can_cut used to plan Cerulean -> Route 9 -> Cerulean and
// stall. Cerulean -> Route 9 is ordinary geometry; the router must still
// refuse a no-op bounce through Route 9.
func TestGoToRoute4FromCeruleanEastDoesNotBounceRoute9(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM required")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatal(err)
	}
	var mem state.Mem
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade | 1<<state.BadgeThunder | 1<<state.BadgeRainbow
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 6
	copy(mem.Slice(base+sym.MonMoves, 4), []byte{cutMove, 0, 0, 0})
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if !prereqs.Capabilities.Has(capCanCut) {
		t.Fatalf("capabilities did not include can_cut: %v", prereqs.Capabilities)
	}

	dest, ok := Place("route 4")
	if !ok {
		t.Fatal("route 4 place missing")
	}
	plan, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, semanticCeruleanCityMap, dest.Map, 39, 17, int(dest.X), int(dest.Y), nil, prereqs,
	)
	if err != nil {
		return
	}
	if len(plan) >= 2 && plan[0].Edge.To == semanticRoute9Map && plan[1].Edge.To == semanticCeruleanCityMap {
		t.Fatalf("Cerulean east planned Route 9 bounce toward Route 4: %+v", plan)
	}
}
