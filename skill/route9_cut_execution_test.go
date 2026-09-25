package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

// TestRoute9CutExecutorOwnsTheTree documents the contract that failed in
// run-os1jmuuqpc1033zjhq2at3qz4: red:route9_cut must not be a capability-only
// no-op when leaving toward Route 10. The tree that joins Route 9's walking
// components lives inside the map, so the semantic executor clears it there.
// Cerulean -> Route 9 stays ordinary geometry.
func TestRoute9CutExecutorOwnsTheTree(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeConnection, From: semanticRoute9Map, To: route10Map}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok {
		t.Fatal("Route 9 -> Route 10 missing red:route9_cut")
	}
	if transition.ID != "red:route9_cut" || transition.Gate || !transition.PivotOnly || !transition.PortBypass {
		t.Fatalf("transition=%+v, want PivotOnly+PortBypass red:route9_cut", transition)
	}
	if len(transition.Requires) != 1 || transition.Requires[0] != capCanCut {
		t.Fatalf("requires=%v, want [%s]", transition.Requires, capCanCut)
	}
	enter := world.Edge{Kind: world.EdgeConnection, From: semanticCeruleanCityMap, To: semanticRoute9Map}
	if _, ok := redRouteTransitionForEdge(enter); ok {
		t.Fatal("Cerulean -> Route 9 must remain ordinary geometry, not a Cut pivot")
	}
}
