package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeBattleRuntimeProjectsBoundaryState(t *testing.T) {
	var mem fakeMemory
	mem[sym.IsInBattle] = 2
	mem[sym.CurMap] = 0x2a
	mem[sym.XCoord] = 7
	mem[sym.YCoord] = 11
	mem[sym.FontLoaded] = 1
	mem[sym.CurrentMenuItem] = 1
	mem[sym.MaxMenuItem] = 3

	got := New().DecodeBattleRuntime(&mem)
	if !got.InBattle {
		t.Fatal("InBattle=false want true")
	}
	if got.NativeMapID != 0x2a || got.X != 7 || got.Y != 11 {
		t.Fatalf("location=%#x (%d,%d), want %#x (7,11)", got.NativeMapID, got.X, got.Y, 0x2a)
	}
	if !got.TextActive {
		t.Fatal("TextActive=false want true")
	}
	if got.MenuCursor.Current != 1 || got.MenuCursor.Max != 3 {
		t.Fatalf("cursor=%+v want current=1 max=3", got.MenuCursor)
	}
}
