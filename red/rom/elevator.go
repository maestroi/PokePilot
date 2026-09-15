package rom

// ElevatorFloor describes one selectable floor in a Red elevator menu.
// DestWarpID is the zero-based destination-warp index written directly into
// wWarpEntries by DisplayElevatorFloorMenu.
type ElevatorFloor struct {
	MapID      uint8
	DestWarpID uint8
}

// ElevatorSpec is adapter-owned Red ROM knowledge for an elevator map. The
// panel coordinate is a background event the player must face and activate;
// Floors are in the exact order displayed by the game's special-list menu.
type ElevatorSpec struct {
	PanelX uint8
	PanelY uint8
	Floors []ElevatorFloor
}

var redElevators = map[uint8]ElevatorSpec{
	// CELADON_MART_ELEVATOR. pokered/scripts/CeladonMartElevator.asm:
	// 1F, 2F, 3F, 4F, 5F.
	0x7F: {
		PanelX: 3,
		PanelY: 0,
		Floors: []ElevatorFloor{
			{MapID: 0x7A, DestWarpID: 5},
			{MapID: 0x7B, DestWarpID: 2},
			{MapID: 0x7C, DestWarpID: 2},
			{MapID: 0x7D, DestWarpID: 2},
			{MapID: 0x88, DestWarpID: 2},
		},
	},

	// ROCKET_HIDEOUT_ELEVATOR. B3F is intentionally absent from the menu.
	0xCB: {
		PanelX: 1,
		PanelY: 1,
		Floors: []ElevatorFloor{
			{MapID: 0xC7, DestWarpID: 4},
			{MapID: 0xC8, DestWarpID: 4},
			{MapID: 0xCA, DestWarpID: 2},
		},
	},

	// SILPH_CO_ELEVATOR. pokered/scripts/SilphCoElevator.asm: 1F..11F.
	0xEC: {
		PanelX: 3,
		PanelY: 0,
		Floors: []ElevatorFloor{
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
		},
	},
}

// LookupElevator returns the Red elevator definition for mapID. The returned
// floor slice is copied so callers cannot mutate adapter facts globally.
func LookupElevator(mapID uint8) (ElevatorSpec, bool) {
	spec, ok := redElevators[mapID]
	if !ok {
		return ElevatorSpec{}, false
	}
	spec.Floors = append([]ElevatorFloor(nil), spec.Floors...)
	return spec, true
}

// ElevatorFloorForDestination resolves a graph destination to the exact menu
// entry and live destination-warp id the Red elevator script will install.
func ElevatorFloorForDestination(elevatorMap, destinationMap uint8) (ElevatorSpec, ElevatorFloor, int, bool) {
	spec, ok := LookupElevator(elevatorMap)
	if !ok {
		return ElevatorSpec{}, ElevatorFloor{}, 0, false
	}
	for i, floor := range spec.Floors {
		if floor.MapID == destinationMap {
			return spec, floor, i, true
		}
	}
	return ElevatorSpec{}, ElevatorFloor{}, 0, false
}
