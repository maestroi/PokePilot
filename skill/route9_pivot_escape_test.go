package skill

import (
	"testing"

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
