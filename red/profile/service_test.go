package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeShopActionMenuBeatsGenericControllableState(t *testing.T) {
	var mem fakeMemory
	// state.Controllable only models overworld transition/battle locks; it does
	// not include menu ownership. This is the farm #1958 shape: the mart's
	// BUY/SELL/QUIT registers are live while the generic predicate can already
	// read as controllable.
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 8
	mem[sym.MenuWatchedKeys] = shopWatchActionMenu
	mem[sym.MaxMenuItem] = shopActionMenuMax

	got := New().DecodeShop(&mem)
	if !got.Controllable {
		t.Fatal("regression setup no longer exposes the generic-controllable/menu overlap")
	}
	if got.Phase != game.ShopPhaseActionMenu {
		t.Fatalf("DecodeShop phase = %d, want action menu despite generic controllable=true", got.Phase)
	}
}

func TestDecodeShopClosedWhenNoShopSurfaceOwnsInput(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 8

	got := New().DecodeShop(&mem)
	if !got.Controllable {
		t.Fatal("closed-overworld setup should be controllable")
	}
	if got.Phase != game.ShopPhaseClosed {
		t.Fatalf("DecodeShop phase = %d, want closed overworld", got.Phase)
	}
}
