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

func TestDecodeShopItemListKeepsDecodedStock(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 8
	mem[sym.MenuWatchedKeys] = shopWatchListOrQty
	mem[sym.ItemList] = 2
	mem[sym.ItemList+1] = 0x0b
	mem[sym.ItemList+2] = 0x0c
	mem[sym.ItemList+3] = 0xff
	drawTestShopMenuCursor(&mem, 42)

	got := New().DecodeShop(&mem)
	if got.Phase != game.ShopPhaseItemList {
		t.Fatalf("DecodeShop phase = %d, want item list", got.Phase)
	}
	if len(got.Items) != 2 || got.Items[0] != 0x0b || got.Items[1] != 0x0c {
		t.Fatalf("DecodeShop items = %#v, want [0x0b 0x0c]", got.Items)
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

func TestDecodeShopItemListIgnoresStalePurchaseQuantity(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 8
	mem[sym.MenuWatchedKeys] = shopWatchListOrQty
	mem[sym.ListMenuID] = shopPricedListMenuID
	mem[sym.ItemQuantity] = 2 // left behind by the previous purchase
	mem[sym.MaxItemQuantity] = 99
	drawTestShopMenuCursor(&mem, 42)

	if got := New().DecodeShop(&mem); got.Phase != game.ShopPhaseItemList {
		t.Fatalf("DecodeShop phase = %d, want item list; quantity registers outlive the quantity box (#1958)", got.Phase)
	}

	mem[sym.TileMap+10*shopScreenWidth+8] = shopQuantityGlyph
	if got := New().DecodeShop(&mem); got.Phase != game.ShopPhaseQuantity {
		t.Fatalf("DecodeShop phase = %d, want quantity once the box's × is drawn", got.Phase)
	}
}

func TestDecodeShopPricePromptMoreTextIsPageable(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 8
	mem[sym.MenuWatchedKeys] = shopWatchListOrQty
	mem[sym.ListMenuID] = shopPricedListMenuID
	mem[sym.ItemQuantity] = 2
	mem[sym.MaxItemQuantity] = 99
	drawTestShopMenuCursor(&mem, 42)
	mem[sym.TileMap+10*shopScreenWidth+8] = shopQuantityGlyph
	mem[sym.TileMap+16*shopScreenWidth+18] = shopMoreTextGlyph

	if got := New().DecodeShop(&mem); got.Phase != game.ShopPhaseGreeting {
		t.Fatalf("DecodeShop phase = %d, want pageable text while ▼ awaits a button", got.Phase)
	}
}
