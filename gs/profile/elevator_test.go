package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGoldenrodElevatorMenuAndDoorReadiness(t *testing.T) {
	elevator := mapByName(t, "GOLDENROD_DEPT_STORE_ELEVATOR")
	b1f := mapByName(t, "GOLDENROD_DEPT_STORE_B1F")
	lobby := mapByName(t, "GOLDENROD_DEPT_STORE_1F")
	var mem fakeMemory
	mem[sym.MapGroup], mem[sym.MapNumber] = elevator.Group, elevator.Number
	mem[sym.BackupMapGroup], mem[sym.BackupMapNumber] = b1f.Group, b1f.Number
	toLobby := game.ElevatorTransition{
		SourceMapID:      gsdata.NativeMapID(elevator.Group, elevator.Number),
		DestinationMapID: gsdata.NativeMapID(lobby.Group, lobby.Number),
		DestinationWarp:  3,
	}
	p := NewGold()
	if p.ElevatorTransitionReady(&mem, toLobby) {
		t.Fatal("doors boarded from B1F must not read as armed for 1F")
	}

	// Elevator_AskWhichFloor polling: script running, floor menu header live.
	mem[sym.ScriptRunning] = 1
	mem[sym.CurElevatorCount] = 7
	mem[sym.MenuBorderTopCoord], mem[sym.MenuBorderLeftCoord] = 1, 12
	mem[sym.MenuBorderBottomCoord], mem[sym.MenuBorderRightCoord] = 9, 18
	mem[sym.TwoDMenuNumRows], mem[sym.TwoDMenuNumCols] = 4, 1
	mem[sym.MenuCursorY] = 2
	state := p.DecodeListMenu(&mem)
	if !state.Visible || state.Kind != game.ListMenuElevator || state.Position != 1 {
		t.Fatalf("floor menu = %+v, want elevator entry 1", state)
	}
	mem[sym.MenuBorderLeftCoord] = 0 // standard header while "Which floor?" prints
	if p.DecodeListMenu(&mem).Kind == game.ListMenuElevator {
		t.Fatal("text-box header must not read as the floor menu")
	}

	mem[sym.BackupMapGroup], mem[sym.BackupMapNumber] = lobby.Group, lobby.Number
	if !p.ElevatorTransitionReady(&mem, toLobby) {
		t.Fatal("Elevator_GoToFloor wrote 1F into wBackupMap*, doors should be armed")
	}
	mem[sym.MapGroup], mem[sym.MapNumber] = lobby.Group, lobby.Number
	toLobby.SourceMapID = gsdata.NativeMapID(lobby.Group, lobby.Number)
	if p.ElevatorTransitionReady(&mem, toLobby) {
		t.Fatal("a non-elevator map cannot have armed elevator doors")
	}
}
