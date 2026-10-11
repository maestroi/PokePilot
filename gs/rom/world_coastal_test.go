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
