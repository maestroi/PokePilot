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
