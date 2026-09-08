package world

import "testing"

const (
	museum1FMapForTest = uint8(0x34)
	museum2FMapForTest = uint8(0x35)
)

// TestPewterReverseBanDoesNotTransitMuseum is the farm loop: arriving in
// Pewter from Route 3 reverse-bans the east connection, and the old edge-keyed
// search then used a Pewter building as a way to "reset" the previous map.
// The router now keys known states by map+walkable-component, so returning to
// the same plaza component is a no-op and cannot clear the first-hop ban.
func TestPewterReverseBanDoesNotTransitMuseum(t *testing.T) {
	g := loadGraph(t)
	pewter, route3, cerulean := uint8(0x02), uint8(0x0e), uint8(0x03)
	blocked := map[Edge]bool{}
	for _, e := range g.Edges[pewter] {
		if e.To == route3 {
			blocked[e] = true
		}
	}
	route, err := FindRouteAt(g, pewter, cerulean, 39, 17, blocked)
	if err == nil {
		for _, indoor := range []uint8{museum1FMapForTest, museum2FMapForTest, 0x36, 0x3a} { // museum, gym, pewter center
			if routeVisits(route, indoor) {
				t.Fatalf("Pewter(39,17)->Cerulean with Route 3 banned transited %#04x: %+v", indoor, route)
			}
		}
	}
}

func TestPewterStillRoutesIntoMuseumWhenItIsTheDestination(t *testing.T) {
	g := loadGraph(t)
	route, err := FindRoute(g, 0x02, museum1FMapForTest)
	if err != nil {
		t.Fatalf("FindRoute(Pewter, Museum 1F): %v", err)
	}
	if !routeVisits(route, museum1FMapForTest) {
		t.Fatalf("route to the museum did not enter it: %+v", route)
	}
}

// TestRoute4WestToCeruleanUsesMtMoon is the last-hour farm death after #130:
// Place("route 4") is (10,10) in the west pocket, whose east edge is a
// different component. The south-edge port used to include both the real
// Route 3 seam and the east pocket's unused south tiles, so a Route 3 bounce
// looked like it unlocked Cerulean. Travel then walked into Pewter's Museum.
// The west pocket's only way east is through Mt. Moon.
func TestRoute4WestToCeruleanUsesMtMoon(t *testing.T) {
	g := loadGraph(t)
	route, err := FindRouteAtDestination(g, 0x0f, 0x03, 10, 10, 5, 18, nil)
	if err != nil {
		t.Fatalf("Route 4 (10,10) -> Cerulean (5,18): %v", err)
	}
	if !routeVisits(route, 0x3b) && !routeVisits(route, 0x3c) && !routeVisits(route, 0x3d) {
		t.Fatalf("Route 4 west -> Cerulean did not use Mt. Moon: %+v", route)
	}
	if routeVisits(route, 0x02) || routeVisits(route, museum1FMap) {
		t.Fatalf("Route 4 west -> Cerulean detoured through Pewter: %+v", route)
	}
}

// TestRoute4CaveExitReachesEastEdge is the hop after Mt. Moon: (24,5) is
// the B1F landing, (89,4) is the walkable Cerulean seam. A ledge around
// x=60 used to split them, so the graph never offered the cave as a way
// east.
func TestRoute4CaveExitSharesCeruleanSeam(t *testing.T) {
	g := loadGraph(t)
	c := g.comps[0x0f]
	if c == nil || c[5][24] == 0 {
		t.Fatal("Route 4 cave exit (24,5) is not walkable")
	}
	east := Edge{Kind: EdgeConnection, From: 0x0f, To: 0x03, Dir: dirEast}
	if !shareComp([]int{c[5][24]}, g.exitComps[east]) {
		t.Fatalf("cave exit comp %d does not share the Cerulean seam %v", c[5][24], g.exitComps[east])
	}
}
