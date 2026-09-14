package world

import "testing"

// A map connection covers only the seam the two maps share, offset by the
// coordinate the ROM stores with it. Route 4 is one map with two halves that
// no walk joins: the Route 3 connection lands on the western half, next to
// Mt. Moon's entrance, and the Cerulean half is reachable only through the
// cave. Before the offset was read, the landing looked like the whole eastern
// edge, so a run that stepped back out of Mt. Moon believed it could walk to
// Cerulean and turned around forever.
func TestConnectionLandingDoesNotReachUnrelatedEdgeRegion(t *testing.T) {
	g := loadGraph(t)
	landingSet := map[int]bool{}
	for _, e := range g.Edges[0x0e] {
		if e.Kind != EdgeConnection || e.To != 0x0f {
			continue
		}
		// A ROM connection may now be represented by several component-scoped
		// bands, including solid bands with no ordinary walking component. The
		// old test picked the first edge, which became order-dependent after the
		// split. Aggregate only the immediate seam landing components across all
		// bands; directed reachability is a separate routing concern.
		for _, comp := range g.connectionPortComps(e, true) {
			landingSet[comp] = true
		}
	}
	landing := make([]int, 0, len(landingSet))
	for comp := range landingSet {
		landing = append(landing, comp)
	}
	west := componentSetAt(g, 0x0f, 10, 10)
	east := componentSetAt(g, 0x0f, 64, 14)
	if !shareComp(landing, west) {
		t.Fatalf("Route 3 must land on western Route 4: landing=%v west=%v", landing, west)
	}
	if shareComp(landing, east) {
		t.Fatalf("Route 3 connection falsely lands in eastern Route 4: landing=%v east=%v", landing, east)
	}

	route, err := FindRouteAtDestination(g, 0x0f, 0x03, 10, 10, 5, 18, nil)
	if err != nil {
		t.Fatalf("FindRouteAtDestination(Route 4 west -> Cerulean): %v", err)
	}
	for _, leg := range route {
		if leg.From == 0x0f && leg.To == 0x0e {
			t.Fatalf("route to Cerulean falsely leaves toward Route 3: %+v", route)
		}
	}
}
