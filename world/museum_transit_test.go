package world

import "testing"

func TestPewterBuildingIsTransitOnlyWhenNotTheDestination(t *testing.T) {
	tests := []struct {
		hop, dest uint8
		want      bool
	}{
		{hop: museum1FMap, dest: 0x03, want: true}, // Cerulean
		{hop: museum2FMap, dest: 0x0e, want: true}, // Route 3
		{hop: 0x36, dest: 0x03, want: true},        // Pewter Gym
		{hop: 0x3a, dest: 0x03, want: true},        // Pewter Center
		{hop: museum1FMap, dest: museum1FMap, want: false},
		{hop: museum1FMap, dest: museum2FMap, want: false},
		{hop: museum2FMap, dest: museum1FMap, want: false},
		{hop: 0x36, dest: 0x36, want: false},
		{hop: 0x02, dest: 0x03, want: false},
		{hop: 0x44, dest: 0x03, want: false}, // Route 4 center is a real component-change
		{hop: 0x33, dest: 0x02, want: false}, // Viridian Forest
	}
	for _, tt := range tests {
		if got := pewterBuildingIsTransit(tt.hop, tt.dest); got != tt.want {
			t.Errorf("pewterBuildingIsTransit(%#04x, %#04x) = %v, want %v", tt.hop, tt.dest, got, tt.want)
		}
	}
}

// TestPewterReverseBanDoesNotTransitMuseum is the farm loop: arriving in
// Pewter from Route 3 reverse-bans the east connection, and the old search
// then used the Museum warp as a way to "reset" the previous map. That walk
// is what put every Cerulean journey on the ticket YES/NO.
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
		for _, indoor := range []uint8{museum1FMap, museum2FMap, 0x36, 0x3a} { // museum, gym, pewter center
			if routeVisits(route, indoor) {
				t.Fatalf("Pewter(39,17)->Cerulean with Route 3 banned transited %#04x: %+v", indoor, route)
			}
		}
	}
}

func TestPewterStillRoutesIntoMuseumWhenItIsTheDestination(t *testing.T) {
	g := loadGraph(t)
	route, err := FindRoute(g, 0x02, museum1FMap)
	if err != nil {
		t.Fatalf("FindRoute(Pewter, Museum 1F): %v", err)
	}
	if !routeVisits(route, museum1FMap) {
		t.Fatalf("route to the museum did not enter it: %+v", route)
	}
}
