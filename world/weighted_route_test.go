package world

import (
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

type weightedRouteTestProvider struct {
	width  int
	height int
}

func (p weightedRouteTestProvider) MapIDs() []uint8 { return []uint8{1, 2, 4} }

func (p weightedRouteTestProvider) ParseMap(mapID uint8) (worldmodel.MapHeader, error) {
	return worldmodel.MapHeader{
		ID:           mapID,
		WidthBlocks:  uint8(p.width / 2),
		HeightBlocks: uint8(p.height / 2),
	}, nil
}

func (p weightedRouteTestProvider) Grid(mapID uint8, _ []byte, _ worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	n := p.width * p.height
	walkable := make([]bool, n)
	for i := range walkable {
		walkable[i] = true
	}
	return worldmodel.GridSpec{
		MapID:         mapID,
		Width:         p.width,
		Height:        p.height,
		Walkable:      walkable,
		CollisionTile: make([]uint8, n),
		FieldTile:     make([]uint8, n),
	}, nil
}

func (weightedRouteTestProvider) LookupElevator(uint8) (worldmodel.ElevatorSpec, bool) {
	return worldmodel.ElevatorSpec{}, false
}

func (weightedRouteTestProvider) ElevatorFloorForDestination(uint8, uint8) (worldmodel.ElevatorFloor, bool) {
	return worldmodel.ElevatorFloor{}, false
}

func TestWeightedRouteCanPreferMoreTransitionsWhenWalkingIsShorter(t *testing.T) {
	direct := Edge{Kind: EdgeWarp, From: 1, To: 4, WarpX: 18, WarpY: 1}
	via := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 1, WarpY: 1}
	finish := Edge{Kind: EdgeWarp, From: 2, To: 4, WarpX: 1, WarpY: 1}

	provider := weightedRouteTestProvider{width: 20, height: 4}
	g := &Graph{
		Edges: map[uint8][]Edge{
			1: {direct, via},
			2: {finish},
			4: nil,
		},
		warps: map[uint8][]worldmodel.Warp{
			1: {
				{X: 18, Y: 1, DestWarpID: 0, DestMap: 4},
				{X: 1, Y: 1, DestWarpID: 0, DestMap: 2},
			},
			2: {
				{X: 0, Y: 1, DestWarpID: 0, DestMap: 1},
				{X: 1, Y: 1, DestWarpID: 1, DestMap: 4},
			},
			4: {
				{X: 19, Y: 1, DestWarpID: 0, DestMap: 1},
				{X: 0, Y: 1, DestWarpID: 1, DestMap: 2},
			},
		},
		tiles: map[uint8]dim{
			1: {w: 20, h: 4},
			2: {w: 20, h: 4},
			4: {w: 20, h: 4},
		},
		provider: provider,
	}

	bfs, err := FindRoute(g, 1, 4)
	if err != nil {
		t.Fatalf("FindRoute: %v", err)
	}
	if len(bfs) != 1 || bfs[0] != direct {
		t.Fatalf("BFS route = %+v, want one-hop direct route", bfs)
	}

	result, err := FindWeightedRoutePlanAtDestinationWithCapabilities(
		g, 1, 4, 0, 1, -1, -1, nil, RoutePrerequisites{}, DefaultRouteCostPolicy(),
	)
	if err != nil {
		t.Fatalf("weighted route: %v", err)
	}
	if !result.Exact {
		t.Fatalf("weighted route unexpectedly fell back: %+v", result)
	}
	if len(result.Steps) != 2 || result.Steps[0].Edge != via || result.Steps[1].Edge != finish {
		t.Fatalf("weighted route = %+v, want short walking route 1->2->4", result.Steps)
	}
}

func TestWeightedRouteFallsBackWhenGeometryIsUnavailable(t *testing.T) {
	direct := Edge{Kind: EdgeConnection, From: 1, To: 3}
	via := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 1, WarpY: 1}
	g := &Graph{Edges: map[uint8][]Edge{
		1: {direct, via},
		2: {{Kind: EdgeWarp, From: 2, To: 3, WarpX: 1, WarpY: 1}},
		3: nil,
	}}

	result, err := FindWeightedRoutePlanAtDestinationWithCapabilities(
		g, 1, 3, 0, 0, -1, -1, nil, RoutePrerequisites{}, DefaultRouteCostPolicy(),
	)
	if err != nil {
		t.Fatalf("weighted route fallback: %v", err)
	}
	if result.Exact {
		t.Fatalf("result.Exact = true, want conservative fallback without geometry")
	}
	if len(result.Steps) != 1 || result.Steps[0].Edge != direct {
		t.Fatalf("fallback route = %+v, want established BFS direct route", result.Steps)
	}
}

func TestWeightedMapOnlyGoalDoesNotInventDestinationTile(t *testing.T) {
	g := &Graph{Edges: map[uint8][]Edge{1: nil}}
	result, err := FindWeightedRoutePlanAtDestinationWithCapabilities(
		g, 1, 1, 17, 23, -1, -1, nil, RoutePrerequisites{}, DefaultRouteCostPolicy(),
	)
	if err != nil {
		t.Fatalf("map-only weighted route: %v", err)
	}
	if !result.Exact || len(result.Steps) != 0 || result.Cost != 0 {
		t.Fatalf("map-only result = %+v, want exact zero-cost arrival on current map", result)
	}
}
