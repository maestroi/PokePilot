package world

import (
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

type splitPortProvider struct{}

func (splitPortProvider) MapIDs() []worldmodel.MapID { return []worldmodel.MapID{1, 2} }
func (splitPortProvider) ParseMap(mapID worldmodel.MapID) (worldmodel.MapHeader, error) {
	return worldmodel.MapHeader{ID: mapID, WidthBlocks: 2, HeightBlocks: 2}, nil
}
func (splitPortProvider) Grid(mapID worldmodel.MapID, _ []byte, _ worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	const width, height = 4, 4
	walkable := make([]bool, width*height)
	for i := range walkable {
		walkable[i] = true
	}
	if mapID == 1 {
		// Split the source into west/east components. Water traversal keeps the
		// same solid barrier, modeling an island/cave wall that Surf cannot
		// teleport through.
		for y := 0; y < height; y++ {
			walkable[y*width+2] = false
		}
	}
	return worldmodel.GridSpec{
		MapID:         mapID,
		Width:         width,
		Height:        height,
		Walkable:      walkable,
		CollisionTile: make([]uint8, width*height),
		FieldTile:     make([]uint8, width*height),
	}, nil
}
func (splitPortProvider) LookupElevator(worldmodel.MapID) (worldmodel.ElevatorSpec, bool) {
	return worldmodel.ElevatorSpec{}, false
}
func (splitPortProvider) ElevatorFloorForDestination(worldmodel.MapID, worldmodel.MapID) (worldmodel.ElevatorFloor, bool) {
	return worldmodel.ElevatorFloor{}, false
}

func TestEdgePortReachableFromRejectsDifferentSourceComponent(t *testing.T) {
	near := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 1, WarpY: 1}
	far := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 3, WarpY: 1}
	g := &Graph{
		Edges: map[uint8][]Edge{1: {near, far}, 2: nil},
		warps: map[uint8][]worldmodel.Warp{
			1: {
				{X: 1, Y: 1, DestWarpID: 0, DestMap: 2},
				{X: 3, Y: 1, DestWarpID: 0, DestMap: 2},
			},
			2: {
				{X: 0, Y: 1, DestWarpID: 0, DestMap: 1},
			},
		},
		tiles:    map[uint8]dim{1: {w: 4, h: 4}, 2: {w: 4, h: 4}},
		provider: splitPortProvider{},
	}
	policy := DefaultRouteCostPolicy()
	policy.AllowWater = true

	if !EdgePortReachableFrom(g, 1, 0, 1, near, policy) {
		t.Fatal("near port should be reachable from the west component")
	}
	if EdgePortReachableFrom(g, 1, 0, 1, far, policy) {
		t.Fatal("far port across a solid component barrier must not be reachable even with water traversal")
	}
}
