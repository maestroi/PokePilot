package world

import (
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

type fakeNativeElevatorProvider struct {
	fakeNativeTopologyProvider
	elevators map[uint16]worldmodel.NativeElevatorSpec
}

func (p fakeNativeElevatorProvider) LookupNativeElevator(id uint16) (worldmodel.NativeElevatorSpec, bool) {
	spec, ok := p.elevators[id]
	return spec, ok
}

func TestBuildNativeGraphExpandsDynamicElevatorDoorsPerFloor(t *testing.T) {
	const (
		basement uint16 = 0x0301
		lobby    uint16 = 0x0302
		roof     uint16 = 0x0303 // a floor outside the provider's slice
		car      uint16 = 0x0b01
	)
	topology := fakeNativeTopologyProvider{
		basement: {ID: basement, Warps: []worldmodel.NativeWarp{{X: 9, Y: 4, DestMap: car}}},
		lobby:    {ID: lobby, Warps: []worldmodel.NativeWarp{{X: 2, Y: 0, DestMap: car}}},
		car: {ID: car, Warps: []worldmodel.NativeWarp{
			{X: 1, Y: 3, DestMap: lobby, Dynamic: true},
			{X: 2, Y: 3, DestMap: lobby, Dynamic: true},
		}},
	}
	floors := worldmodel.NativeElevatorSpec{PanelX: 3, Floors: []worldmodel.NativeElevatorFloor{
		{MapID: basement, DestWarpID: 1}, {MapID: lobby, DestWarpID: 3}, {MapID: roof, DestWarpID: 2},
	}}

	plain, err := BuildNativeGraph(topology)
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.Edges[car]; len(got) != 0 {
		t.Fatalf("dynamic doors without a floor list became edges: %+v", got)
	}

	graph, err := BuildNativeGraph(fakeNativeElevatorProvider{topology, map[uint16]worldmodel.NativeElevatorSpec{car: floors}})
	if err != nil {
		t.Fatal(err)
	}
	type door struct {
		x, to uint16
		warp  uint8
	}
	got := map[door]bool{}
	for _, e := range graph.Edges[car] {
		got[door{uint16(e.WarpX), e.To, e.DestWarp}] = true
	}
	want := []door{{1, basement, 1}, {1, lobby, 3}, {2, basement, 1}, {2, lobby, 3}}
	if len(got) != len(want) {
		t.Fatalf("elevator edges = %+v, want %+v", graph.Edges[car], want)
	}
	for _, d := range want {
		if !got[d] {
			t.Fatalf("missing elevator edge %+v in %+v", d, graph.Edges[car])
		}
	}
	route, err := FindNativeRoute(graph, basement, lobby)
	if err != nil || len(route) != 2 || route[1].From != car || route[1].To != lobby {
		t.Fatalf("basement -> lobby via elevator = %+v, %v", route, err)
	}
}
