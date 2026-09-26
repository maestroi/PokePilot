package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/sym"
)

const testShopMenuCursorTile = 0xed

func drawTestShopMenuCursor(mem *fakeMemory, offset uint16) {
	mem[sym.FontLoaded] = 1
	addr := sym.TileMap + offset
	mem[sym.MenuCursorLocation] = byte(addr)
	mem[sym.MenuCursorLocation+1] = byte(addr >> 8)
	mem[addr] = testShopMenuCursorTile
}

func TestDecodeShopIgnoresStaleActionMenuRegistersOnControllableOverworld(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 8
	mem[sym.MenuWatchedKeys] = shopWatchActionMenu
	mem[sym.MaxMenuItem] = shopActionMenuMax

	got := New().DecodeShop(&mem)
	if !got.Controllable {
		t.Fatal("closed-overworld setup should be controllable")
	}
	if got.Phase != game.ShopPhaseClosed {
		t.Fatalf("DecodeShop phase = %d, want closed; stale menu registers are not a live shop surface", got.Phase)
	}
}

func TestDecodeShopGreetingBeatsStaleItemListRegisters(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 8
	mem[sym.FontLoaded] = 1
	mem[sym.MenuWatchedKeys] = shopWatchListOrQty
	mem[sym.ItemQuantity] = 1
	mem[sym.MaxItemQuantity] = 99

	got := New().DecodeShop(&mem)
	if got.Controllable {
		t.Fatal("greeting with FontLoaded set must not be controllable")
	}
	if got.Phase != game.ShopPhaseGreeting {
		t.Fatalf("DecodeShop phase = %d, want greeting; stale list/quantity registers have no drawn cursor", got.Phase)
	}
}

func TestDecodeShopRecognizesLiveActionMenuCursor(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 8
	mem[sym.MenuWatchedKeys] = shopWatchActionMenu
	mem[sym.MaxMenuItem] = shopActionMenuMax
	drawTestShopMenuCursor(&mem, 42)

	got := New().DecodeShop(&mem)
	if got.Phase != game.ShopPhaseActionMenu {
		t.Fatalf("DecodeShop phase = %d, want action menu with live cursor", got.Phase)
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
