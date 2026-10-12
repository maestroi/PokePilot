package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

// This is the exact missing live-routing slice between the known fourth-badge
// boundary and the coastal six-badge frontier. The full ROM world parser is
// a separate diagnostic API; GoToNative consumes NewFirstBadgeWorldProvider.
func TestCoastalNativeTopologyExposesEveryLiveMapAndCollision(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	for _, name := range []string{
		"ROUTE_38_ECRUTEAK_GATE", "ROUTE_38", "ROUTE_39",
		"OLIVINE_CITY", "OLIVINE_POKECENTER_1F", "OLIVINE_CAFE",
		"OLIVINE_GYM",
		"OLIVINE_LIGHTHOUSE_1F", "OLIVINE_LIGHTHOUSE_2F",
		"OLIVINE_LIGHTHOUSE_3F", "OLIVINE_LIGHTHOUSE_4F",
		"OLIVINE_LIGHTHOUSE_5F", "OLIVINE_LIGHTHOUSE_6F",
		"ROUTE_40", "ROUTE_41",
		"CIANWOOD_CITY", "CIANWOOD_POKECENTER_1F",
		"CIANWOOD_PHARMACY", "CIANWOOD_GYM",
	} {
		t.Run(name, func(t *testing.T) {
			id := nativeID(t, name)
			header, err := provider.ParseMap(id)
			if err != nil {
				t.Fatal(err)
			}
			if header.WidthBlocks == 0 || header.HeightBlocks == 0 {
				t.Fatalf("empty native map header for %s: %+v", name, header)
			}
			blocks := make([]byte, int(header.WidthBlocks)*int(header.HeightBlocks))
			if _, err := provider.Grid(id, blocks, 0); err != nil {
				t.Fatalf("native collision grid for %s: %v", name, err)
			}
		})
	}
}

func TestCoastalNativeTopologyRoutesOlivineCianwoodAndReturn(t *testing.T) {
	graph, err := world.BuildNativeGraph(NewFirstBadgeWorldProvider(nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{
		{"ECRUTEAK_CITY", "ROUTE_38", "ROUTE_39", "OLIVINE_CITY", "ROUTE_40", "ROUTE_41", "CIANWOOD_CITY"},
		{"CIANWOOD_CITY", "ROUTE_41", "ROUTE_40", "OLIVINE_CITY"},
		{"OLIVINE_CITY", "OLIVINE_CAFE"},
		{"OLIVINE_CITY", "OLIVINE_GYM"},
		{"CIANWOOD_CITY", "CIANWOOD_GYM"},
		{"CIANWOOD_CITY", "CIANWOOD_PHARMACY"},
	} {
		start := nativeID(t, names[0])
		end := nativeID(t, names[len(names)-1])
		route, err := world.FindNativeRoute(graph, start, end)
		if err != nil {
			t.Fatalf("native coast %s -> %s: %v", names[0], names[len(names)-1], err)
		}
		if len(route) != len(names)-1 {
			t.Fatalf("native coast %s -> %s used %d legs, expected %d: %+v",
				names[0], names[len(names)-1], len(route), len(names)-1, route)
		}
		for i, edge := range route {
			if edge.From != nativeID(t, names[i]) || edge.To != nativeID(t, names[i+1]) {
				t.Fatalf("route leg %d: %#04x -> %#04x, want %s -> %s",
					i, edge.From, edge.To, names[i], names[i+1])
			}
		}
	}
}

func TestCoastalNativeTopologyLighthouseFloorsAndRecovery(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{
		{"OLIVINE_CITY", "OLIVINE_LIGHTHOUSE_1F", "OLIVINE_LIGHTHOUSE_2F", "OLIVINE_LIGHTHOUSE_3F", "OLIVINE_LIGHTHOUSE_4F", "OLIVINE_LIGHTHOUSE_5F", "OLIVINE_LIGHTHOUSE_6F"},
		{"OLIVINE_LIGHTHOUSE_6F", "OLIVINE_LIGHTHOUSE_5F", "OLIVINE_LIGHTHOUSE_4F", "OLIVINE_LIGHTHOUSE_3F", "OLIVINE_LIGHTHOUSE_2F", "OLIVINE_LIGHTHOUSE_1F", "OLIVINE_CITY"},
	} {
		route, err := world.FindNativeRoute(graph, nativeID(t, names[0]), nativeID(t, names[len(names)-1]))
		if err != nil {
			t.Fatalf("lighthouse traversal: %v", err)
		}
		if len(route) != len(names)-1 {
			t.Fatalf("lighthouse used %d legs, want %d: %+v", len(route), len(names)-1, route)
		}
		for i, edge := range route {
			if edge.From != nativeID(t, names[i]) || edge.To != nativeID(t, names[i+1]) {
				t.Fatalf("lighthouse leg %d = %#04x -> %#04x, expected %s -> %s",
					i, edge.From, edge.To, names[i], names[i+1])
			}
		}
	}
}

// TestCoastalNativeTopologyLighthouse5FBadPocketReentersClimbStairs pins the
// live geometry that farm run-1bt0xg6hiyxbo hit: the shortest 4F→5F hop lands
// on (3,5), a pocket that cannot walk to the 6F stairs at (9,15). Once that
// entry's only real 5F→6F edge (the stairs) is sealed, routing must leave and
// re-enter through 5F warp 3 / entry 2 at (9,7), which shares the stairs
// component. Pit reverse warps are inert and must not appear as climb options.
func TestCoastalNativeTopologyLighthouse5FBadPocketReentersClimbStairs(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatal(err)
	}
	five := nativeID(t, "OLIVINE_LIGHTHOUSE_5F")
	six := nativeID(t, "OLIVINE_LIGHTHOUSE_6F")
	four := nativeID(t, "OLIVINE_LIGHTHOUSE_4F")
	for _, edge := range graph.Edges[five] {
		if edge.To == six && !(int(edge.WarpX) == 9 && int(edge.WarpY) == 15) {
			t.Fatalf("unexpected 5F→6F climb edge %+v (pit reverses must be inert)", edge)
		}
	}
	var stairs world.NativeEdge
	for _, edge := range graph.Edges[five] {
		if edge.To == six {
			stairs = edge
			break
		}
	}
	if stairs.To != six {
		t.Fatal("missing 5F stairs to 6F")
	}
	bad := map[world.NativeUnreachable]bool{
		{Map: five, Entry: 1, Edge: stairs}: true,
	}
	route, err := world.FindNativeRouteToEntry(graph, five, 1, six, nil, bad)
	if err != nil {
		t.Fatalf("re-enter climb: %v", err)
	}
	if len(route) < 3 {
		t.Fatalf("re-enter climb legs = %d (%+v), want leave/re-enter/stairs", len(route), route)
	}
	if route[0].To != four || route[1].From != four || route[1].To != five || route[1].Entry() != 2 {
		t.Fatalf("re-enter climb = %+v, want 5F→4F then 4F→5F entry 2", route)
	}
	if route[len(route)-1].To != six || int(route[len(route)-1].WarpX) != 9 || int(route[len(route)-1].WarpY) != 15 {
		t.Fatalf("final leg = %+v, want 5F stairs (9,15)→6F", route[len(route)-1])
	}
}

// TestCoastalNativeTopologyLighthousePitFallsRemainOneWay pins the game fact
// that Olivine Lighthouse pit tiles are fall-down only: reverse landings are
// inert so native routing cannot invent climb-the-hole shortcuts.
func TestCoastalNativeTopologyLighthousePitFallsRemainOneWay(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatal(err)
	}
	four := nativeID(t, "OLIVINE_LIGHTHOUSE_4F")
	three := nativeID(t, "OLIVINE_LIGHTHOUSE_3F")
	var fall bool
	for _, edge := range graph.Edges[four] {
		if edge.To == three && int(edge.WarpX) == 8 && int(edge.WarpY) == 3 {
			fall = true
		}
	}
	if !fall {
		t.Fatal("4F pit fall (8,3)→3F must remain routable")
	}
	for _, edge := range graph.Edges[three] {
		if edge.To == four && int(edge.WarpX) == 8 && int(edge.WarpY) == 3 {
			t.Fatalf("3F pit landing (8,3)→4F must be inert, found %+v", edge)
		}
	}
}
