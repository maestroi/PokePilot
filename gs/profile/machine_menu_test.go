package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func putGSPackGraphic(mem *fakeMemory) {
	want := byte(0x50)
	for row := uint16(3); row < 6; row++ {
		for col := uint16(0); col < 5; col++ {
			mem[sym.TileMap+row*20+col] = want
			want++
		}
	}
}

func TestDecodeGoldTMHMPocketAbsolutePosition(t *testing.T) {
	var mem fakeMemory
	putGSPackGraphic(&mem)
	mem[sym.CurPocket] = gen2PocketTMHM
	mem[sym.JumptableIndex] = gen2PackStateTMHMPocketMenu
	mem[sym.TwoDMenuNumRows] = 5
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2TMHMMenuFilter
	mem[sym.MenuCursorY] = 2
	mem[sym.MenuCursorX] = 1
	mem[sym.TMHMPocketScroll] = 1
	mem[sym.TMHMPocketCursor] = 1

	mem[sym.TMsHMs+0] = 1  // TM01
	mem[sym.TMsHMs+1] = 1  // TM02
	mem[sym.TMsHMs+50] = 1 // HM01
	mem[sym.TMsHMs+55] = 1 // HM06

	state := NewGold().DecodeMachineMenu(&mem)
	if !state.Visible || !state.Ready || state.Pocket != game.MachinePocketTMHM {
		t.Fatalf("machine menu=%+v, want visible ready TM/HM pocket", state)
	}
	if state.Position != 2 || state.Count != 4 {
		t.Fatalf("machine menu position/count=%d/%d, want 2/4", state.Position, state.Count)
	}
}

func TestGoldMachineMenuEntryIndexUsesOwnedSparseOrder(t *testing.T) {
	var mem fakeMemory
	mem[sym.TMsHMs+0] = 1  // TM01
	mem[sym.TMsHMs+1] = 1  // TM02
	mem[sym.TMsHMs+50] = 1 // HM01
	mem[sym.TMsHMs+55] = 1 // HM06

	cut, ok := NewGold().NativeFieldMove(game.FieldMoveCut)
	if !ok {
		t.Fatal("Gold profile missing native Cut mapping")
	}
	if got, ok := NewGold().MachineMenuEntryIndex(&mem, cut); !ok || got != 2 {
		t.Fatalf("Cut menu index=%d,%v want 2,true", got, ok)
	}

	fly, ok := NewGold().NativeFieldMove(game.FieldMoveFly)
	if !ok {
		t.Fatal("Gold profile missing native Fly mapping")
	}
	if _, ok := NewGold().MachineMenuEntryIndex(&mem, fly); ok {
		t.Fatal("unowned HM02 was exposed as a menu entry")
	}
}

func TestGoldMachineMenuRequiresLivePackGraphic(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurPocket] = gen2PocketTMHM
	mem[sym.JumptableIndex] = gen2PackStateTMHMPocketMenu
	mem[sym.TwoDMenuNumRows] = 5
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2TMHMMenuFilter
	mem[sym.MenuCursorY] = 1
	mem[sym.MenuCursorX] = 1

	state := NewGold().DecodeMachineMenu(&mem)
	if state.Visible || state.Ready {
		t.Fatalf("stale pack RAM reported live: %+v", state)
	}
}

func TestGoldMachineMenuReportsOtherPackPocketsWithoutPretendingReady(t *testing.T) {
	var mem fakeMemory
	putGSPackGraphic(&mem)
	mem[sym.CurPocket] = gen2PocketKeyItems

	state := NewSilver().DecodeMachineMenu(&mem)
	if !state.Visible || state.Pocket != game.MachinePocketKeyItems || state.Ready || state.Position != -1 {
		t.Fatalf("key-item pocket projection=%+v", state)
	}
}
