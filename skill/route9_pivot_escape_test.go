package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestRoute9CutIsPivotOnlyInBothDirections(t *testing.T) {
	for _, edge := range []world.Edge{
		{Kind: world.EdgeConnection, From: semanticCeruleanCityMap, To: semanticRoute9Map},
		{Kind: world.EdgeConnection, From: semanticRoute9Map, To: semanticCeruleanCityMap},
	} {
		transition, ok := redRouteTransitionForEdge(edge)
		if !ok {
			t.Fatalf("Route 9 edge %+v has no semantic transition", edge)
		}
		if transition.ID != "red:route9_cut" {
			t.Fatalf("Route 9 edge %+v transition id = %q", edge, transition.ID)
		}
		if transition.Gate {
			t.Fatalf("Route 9 edge %+v is a gate; Cut must remain a component pivot", edge)
		}
		if !transition.PivotOnly {
			t.Fatalf("Route 9 edge %+v is not PivotOnly; west-side checkpoints can be stranded without Cut", edge)
		}
		if len(transition.Requires) != 1 || transition.Requires[0] != capCanCut {
			t.Fatalf("Route 9 edge %+v requirements = %v, want [%s]", edge, transition.Requires, capCanCut)
		}
	}
}

// TestGoToRoute4FromCeruleanEastDoesNotBounceRoute9 is farm #1261
// (run-27dtzi7qnqt962i4ecootzj8tg): GoTo Route 4 (10,10) from Cerulean's
// east seam with can_cut used to plan Cerulean -> Route 9 -> Cerulean and
// stall. The adapter still annotates both directions; the generic router
// must not treat that re-entry as a component teleport.
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
