package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/gs/sym"
)

var _ game.ElevatorTransitionDecoder = (*Profile)(nil)

// Elevator_MenuHeader (engine/events/elevator.asm): menu_coords 12, 1, 18, 9.
// The floor menu owns wMenuHeader only while ScrollingMenu polls; the
// "Which floor?" text before it and the window pop after it use other headers.
const (
	gen2ElevatorMenuTop    = 1
	gen2ElevatorMenuLeft   = 12
	gen2ElevatorMenuBottom = 9
	gen2ElevatorMenuRight  = 18
)

// ElevatorTransitionReady reports whether the current elevator's -1 doors
// lead to the requested floor. Elevator_GoToFloor copies the chosen floor into
// wBackupWarpNumber/wBackupMapGroup/wBackupMapNumber; boarding sets the same
// bytes to the arrival floor, so riding to the origin is already armed (its
// landing is the door the player boarded through).
func (*Profile) ElevatorTransitionReady(reader game.MemoryReader, transition game.ElevatorTransition) bool {
	if reader == nil {
		return false
	}
	current := gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber))
	if current != transition.SourceMapID {
		return false
	}
	if _, ok := gsrom.NativeElevator(current); !ok {
		return false
	}
	return gsdata.NativeMapID(reader.Peek8(sym.BackupMapGroup), reader.Peek8(sym.BackupMapNumber)) == transition.DestinationMapID
}

// gsElevatorMenuLive recognizes the floor ScrollingMenu while it polls.
func gsElevatorMenuLive(reader game.MemoryReader) bool {
	if gsControllable(reader) {
		return false
	}
	spec, ok := gsrom.NativeElevator(gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber)))
	if !ok || int(reader.Peek8(sym.CurElevatorCount)) != len(spec.Floors) {
		return false
	}
	if reader.Peek8(sym.MenuBorderTopCoord) != gen2ElevatorMenuTop ||
		reader.Peek8(sym.MenuBorderLeftCoord) != gen2ElevatorMenuLeft ||
		reader.Peek8(sym.MenuBorderBottomCoord) != gen2ElevatorMenuBottom ||
		reader.Peek8(sym.MenuBorderRightCoord) != gen2ElevatorMenuRight {
		return false
	}
	y, _, rows, cols, _ := gsMenuCursor(reader)
	return cols == 1 && rows >= 1 && y >= 1 && y <= rows
}
