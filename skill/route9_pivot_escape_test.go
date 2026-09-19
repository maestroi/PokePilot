package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestRoute9CutIsPivotOnlyWhenLeavingRoute9(t *testing.T) {
	cases := []struct {
		name      string
		edge      world.Edge
		wantPivot bool
	}{
		{
			name:      "Route 9 -> Cerulean",
			edge:      world.Edge{Kind: world.EdgeConnection, From: semanticRoute9Map, To: semanticCeruleanCityMap},
			wantPivot: true,
		},
		{
			name:      "Route 9 -> Route 10",
			edge:      world.Edge{Kind: world.EdgeConnection, From: semanticRoute9Map, To: route10Map},
			wantPivot: true,
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
			if len(transition.Requires) != 1 || transition.Requires[0] != capCanCut {
				t.Fatalf("requirements=%v, want [%s]", transition.Requires, capCanCut)
			}
		})
	}
}
