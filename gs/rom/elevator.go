package rom

import (
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/worldmodel"
)

type elevatorFloorSpec struct {
	warp uint8 // decomp warp ids are 1-based
	dest string
}

type elevatorSpec struct {
	panelX, panelY uint8
	floors         []elevatorFloorSpec
}

// gen2Elevators transcribes each elevator's BGEVENT_READ panel and elevfloor
// table in menu order (pret/pokegold maps/*Elevator.asm).
var gen2Elevators = map[string]elevatorSpec{
	"GOLDENROD_DEPT_STORE_ELEVATOR": {panelX: 3, panelY: 0, floors: []elevatorFloorSpec{
		{2, "GOLDENROD_DEPT_STORE_B1F"},
		{4, "GOLDENROD_DEPT_STORE_1F"},
		{3, "GOLDENROD_DEPT_STORE_2F"},
		{3, "GOLDENROD_DEPT_STORE_3F"},
		{3, "GOLDENROD_DEPT_STORE_4F"},
		{3, "GOLDENROD_DEPT_STORE_5F"},
		{2, "GOLDENROD_DEPT_STORE_6F"},
	}},
}

// NativeElevator returns the floor menu of a Gold/Silver elevator map.
func NativeElevator(mapID uint16) (worldmodel.NativeElevatorSpec, bool) {
	info, ok := gsdata.Map(mapID)
	if !ok {
		return worldmodel.NativeElevatorSpec{}, false
	}
	spec, ok := gen2Elevators[info.Name]
	if !ok {
		return worldmodel.NativeElevatorSpec{}, false
	}
	out := worldmodel.NativeElevatorSpec{PanelX: spec.panelX, PanelY: spec.panelY,
		Floors: make([]worldmodel.NativeElevatorFloor, 0, len(spec.floors))}
	for _, floor := range spec.floors {
		dest, ok := gsdata.MapByName(floor.dest)
		if !ok || floor.warp == 0 {
			return worldmodel.NativeElevatorSpec{}, false
		}
		out.Floors = append(out.Floors, worldmodel.NativeElevatorFloor{
			MapID:      gsdata.NativeMapID(dest.Group, dest.Number),
			DestWarpID: floor.warp - 1,
		})
	}
	return out, true
}

func (p *firstBadgeWorldProvider) LookupNativeElevator(mapID uint16) (worldmodel.NativeElevatorSpec, bool) {
	if p == nil {
		return worldmodel.NativeElevatorSpec{}, false
	}
	if _, ok := p.specs[mapID]; !ok {
		return worldmodel.NativeElevatorSpec{}, false
	}
	return NativeElevator(mapID)
}
