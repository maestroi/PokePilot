package rom

import "github.com/maestroi/pokepilot/worldmodel"

// Yellow keeps the three Gen-I elevator menus on the same native map ids and
// destination-warp slots. These are game-owned script facts, not generic data.
var yellowElevators = map[uint8]worldmodel.ElevatorSpec{
	0x7F: {PanelX: 3, PanelY: 0, Floors: []worldmodel.ElevatorFloor{
		{MapID: 0x7A, DestWarpID: 5},
		{MapID: 0x7B, DestWarpID: 2},
		{MapID: 0x7C, DestWarpID: 2},
		{MapID: 0x7D, DestWarpID: 2},
		{MapID: 0x88, DestWarpID: 2},
	}},
	0xCB: {PanelX: 1, PanelY: 1, Floors: []worldmodel.ElevatorFloor{
		{MapID: 0xC7, DestWarpID: 4},
		{MapID: 0xC8, DestWarpID: 4},
		{MapID: 0xCA, DestWarpID: 2},
	}},
	0xEC: {PanelX: 3, PanelY: 0, Floors: []worldmodel.ElevatorFloor{
		{MapID: 0xB5, DestWarpID: 3},
		{MapID: 0xCF, DestWarpID: 2},
		{MapID: 0xD0, DestWarpID: 2},
		{MapID: 0xD1, DestWarpID: 2},
		{MapID: 0xD2, DestWarpID: 2},
		{MapID: 0xD3, DestWarpID: 2},
		{MapID: 0xD4, DestWarpID: 2},
		{MapID: 0xD5, DestWarpID: 2},
		{MapID: 0xE9, DestWarpID: 2},
		{MapID: 0xEA, DestWarpID: 2},
		{MapID: 0xEB, DestWarpID: 1},
	}},
}

func lookupElevator(mapID uint8) (worldmodel.ElevatorSpec, bool) {
	spec, ok := yellowElevators[mapID]
	if !ok {
		return worldmodel.ElevatorSpec{}, false
	}
	spec.Floors = append([]worldmodel.ElevatorFloor(nil), spec.Floors...)
	return spec, true
}

func elevatorFloorForDestination(elevatorMap, destinationMap uint8) (worldmodel.ElevatorFloor, bool) {
	spec, ok := lookupElevator(elevatorMap)
	if !ok {
		return worldmodel.ElevatorFloor{}, false
	}
	for _, floor := range spec.Floors {
		if floor.MapID == worldmodel.MapID(destinationMap) {
			return floor, true
		}
	}
	return worldmodel.ElevatorFloor{}, false
}
