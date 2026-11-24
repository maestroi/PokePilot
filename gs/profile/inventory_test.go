package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestDecodeGoldInventoryPockets(t *testing.T) {
	var mem fakeMemory
	mem[sym.Money], mem[sym.Money+1], mem[sym.Money+2] = 0x00, 0x57, 0x50 // 22352
	mem[sym.NumItems] = 1
	mem[sym.Items], mem[sym.Items+1] = 0x12, 1
	mem[sym.NumBalls] = 2
	mem[sym.Balls], mem[sym.Balls+1] = 0x05, 5
	mem[sym.Balls+2], mem[sym.Balls+3] = 0x04, 20
	mem[sym.NumKeyItems] = 1
	mem[sym.KeyItems] = 0xaf

	inv := NewGold().DecodeInventory(&mem)
	want := []game.InventoryItem{
		{NativeItemID: 0x12, Quantity: 1, Pocket: game.MachinePocketItems},
		{NativeItemID: 0x05, Quantity: 5, Pocket: game.MachinePocketBalls},
		{NativeItemID: 0x04, Quantity: 20, Pocket: game.MachinePocketBalls},
		{NativeItemID: 0xaf, Quantity: 1, Pocket: game.MachinePocketKeyItems},
	}
	if inv.Money != 22352 || len(inv.Items) != len(want) {
		t.Fatalf("inventory = %+v", inv)
	}
	for i := range want {
		if inv.Items[i] != want[i] {
			t.Fatalf("item %d = %+v, want %+v", i, inv.Items[i], want[i])
		}
	}
}

func TestDecodeGoldPackBallsPocketListPosition(t *testing.T) {
	var mem fakeMemory
	putGSPackGraphic(&mem)
	mem[sym.JumptableIndex] = gen2PackStateBallsPocketMenu
	mem[sym.CurPocket] = gen2PocketBalls
	mem[sym.MenuCursorY] = 2
	mem[sym.MenuScrollPosition] = 1

	list := NewGold().DecodeListMenu(&mem)
	if !list.Visible || list.Kind != game.ListMenuItems || list.Pocket != game.MachinePocketBalls || list.Position != 2 {
		t.Fatalf("list = %+v, want visible balls pocket at 2", list)
	}

	// The pocket bytes persist after the pack closes; the drawn pack is proof.
	var closed fakeMemory
	closed[sym.JumptableIndex] = gen2PackStateBallsPocketMenu
	closed[sym.CurPocket] = gen2PocketBalls
	closed[sym.MenuCursorY] = 1
	if got := NewGold().DecodeListMenu(&closed); got.Visible {
		t.Fatalf("closed pack decoded as %+v", got)
	}
}

func gsMartMemory(jumptable byte, text string) *fakeMemory {
	var mem fakeMemory
	putGSText(&mem, text)
	mem[sym.MartJumptableIndex] = jumptable
	mem[sym.CurMartCount] = 2
	mem[sym.CurMartItems], mem[sym.CurMartItems+1], mem[sym.CurMartItems+2] = 0x04, 0x11, 0xff
	return &mem
}

func TestDecodeGoldMartPhases(t *testing.T) {
	p := NewGold()

	top := gsMartMemory(gen2MartTopMenu, "BUY SELL QUIT Welcome! How may I help you?")
	top[sym.TwoDMenuNumRows], top[sym.TwoDMenuNumCols], top[sym.MenuCursorY] = 3, 1, 1
	if got := p.DecodeShop(top); got.Phase != game.ShopPhaseActionMenu || len(got.Items) != 2 || got.Items[0] != 0x04 {
		t.Fatalf("top menu = %+v", got)
	}

	// After BUY, the jumptable still says TOPMENU while the list loads; a press
	// there would land on the list, so the phase must be busy, not greeting.
	if got := p.DecodeShop(gsMartMemory(gen2MartTopMenu, "")); got.Phase != game.ShopPhaseBusy {
		t.Fatalf("top-menu redraw = %+v, want busy", got.Phase)
	}

	list := gsMartMemory(gen2MartBuy, "22352 GREAT BALL 600 SUPER POTION 700")
	list[sym.TwoDMenuNumRows], list[sym.TwoDMenuNumCols], list[sym.MenuCursorY] = 4, 1, 2
	if got := p.DecodeShop(list); got.Phase != game.ShopPhaseItemList {
		t.Fatalf("item list = %+v", got.Phase)
	}
	if got := p.DecodeListMenu(list); !got.Visible || got.Position != 1 || got.Pocket != game.MachinePocketUnknown {
		t.Fatalf("mart list = %+v, want position 1", got)
	}

	qty := gsMartMemory(gen2MartBuy, "GREAT BALL 600 How many? 02 1200")
	qty[sym.ItemQuantityChange] = 2
	qty[sym.MoneyTemp], qty[sym.MoneyTemp+1], qty[sym.MoneyTemp+2] = 0x00, 0x04, 0xb0
	if got := p.DecodeShop(qty); got.Phase != game.ShopPhaseQuantity || got.Quantity != 2 || got.Total != 1200 {
		t.Fatalf("quantity = %+v", got)
	}

	confirm := gsMartMemory(gen2MartBuy, "YES NO 2 GREAT BALL(S) will be 1200.")
	confirm[sym.TwoDMenuNumRows], confirm[sym.TwoDMenuNumCols] = 2, 1
	confirm[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	confirm[sym.MenuCursorY], confirm[sym.MenuCursorX] = 1, 1
	if got := p.DecodeShop(confirm); got.Phase != game.ShopPhaseConfirmation {
		t.Fatalf("confirmation = %+v", got.Phase)
	}

	if got := p.DecodeShop(gsMartMemory(gen2MartBuy, "Here you are. Thank you!")); got.Phase != game.ShopPhaseGreeting {
		t.Fatalf("thanks box = %+v, want greeting", got.Phase)
	}
	if got := p.DecodeShop(gsMartMemory(gen2MartBuy, "22352 GREAT")); got.Phase != game.ShopPhaseBusy {
		t.Fatalf("list redraw = %+v, want busy", got.Phase)
	}
}

func TestDecodeGoldCaptureParty(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 3
	mem[sym.PartySpecies], mem[sym.PartySpecies+1], mem[sym.PartySpecies+2] = 0x9d, 0xaf, 0x80
	got := NewGold().DecodeCapture(&mem)
	if len(got.PartySpecies) != 3 || got.PartySpecies[2] != 0x80 {
		t.Fatalf("capture = %+v", got)
	}
	if dex, ok := NewGold().CaptureDexNumber(nil, 0x80); !ok || dex != 128 {
		t.Fatalf("dex for Tauros = %d,%v", dex, ok)
	}
}

// GivePoke and catches ask _CaughtAskNicknameText; by the time YES/NO is
// drawn, "Give a nickname to" has scrolled off and only this line remains.
func TestDecodeGoldScrolledNicknamePrompt(t *testing.T) {
	var mem fakeMemory
	putGSText(&mem, "YES NO the CHIKORITA you received?")
	mem[sym.TwoDMenuNumRows], mem[sym.TwoDMenuNumCols] = 2, 1
	mem[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	mem[sym.MenuCursorY], mem[sym.MenuCursorX] = 1, 1
	if got := NewGold().DecodePrompt(&mem); !got.Visible || got.Kind != game.PromptNickname {
		t.Fatalf("prompt = %+v, want nickname", got)
	}
}

// Battle medicine sees only the Items pocket, in native order, so a Bag index
// is the PACK row; the table is Gen-II numbered (0x0e FULL_RESTORE).
func TestDecodeGoldBattleResourcesProjectsItemsPocketMedicine(t *testing.T) {
	var mem fakeMemory
	mem[sym.NumItems] = 2
	mem[sym.Items], mem[sym.Items+1] = 0x90, 1 // non-medicine
	mem[sym.Items+2], mem[sym.Items+3] = 0x0e, 3
	mem[sym.NumBalls] = 1
	mem[sym.Balls], mem[sym.Balls+1] = 0x05, 5

	res := NewGold().DecodeBattleResources(&mem)
	if len(res.Bag) != 2 || res.Bag[1].NativeItemID != 0x0e || res.Bag[1].Pocket != game.MachinePocketItems {
		t.Fatalf("battle bag = %+v, want the two Items-pocket rows only", res.Bag)
	}
	if idx, ok := res.ItemIndex(0x0e); !ok || idx != 1 {
		t.Fatalf("FULL_RESTORE row = %d,%v want 1", idx, ok)
	}
	med, ok := res.Medicine(0x0e)
	if !ok || med.HP != game.MedicineFullHP || !med.CuresStatus("frozen") {
		t.Fatalf("Gen-II 0x0e medicine = %+v,%v want FULL_RESTORE", med, ok)
	}
	if med, ok := res.Medicine(0x10); !ok || med.HP != 200 || med.Cures != "" {
		t.Fatalf("Gen-II 0x10 medicine = %+v,%v want HYPER_POTION", med, ok)
	}
}

// Cycling Road gates turn the player back at x=5 without a BICYCLE; with
// one the gate is ordinary floor.
func TestGoldBicycleGateIsAScriptBarrierWithoutBicycle(t *testing.T) {
	var mem fakeMemory
	gate, _ := gsdata.MapByName("ROUTE_16_GATE")
	mem[sym.MapGroup], mem[sym.MapNumber] = gate.Group, gate.Number
	p := NewGold()
	if got := p.gen2ScriptBarriers(&mem); len(got) != 2 || got[0] != (game.MapPoint{X: 5, Y: 4}) {
		t.Fatalf("barriers without bicycle = %+v", got)
	}
	mem[sym.NumKeyItems] = 1
	mem[sym.KeyItems] = gen2BicycleItem
	if got := p.gen2ScriptBarriers(&mem); got != nil {
		t.Fatalf("barriers with bicycle = %+v, want none", got)
	}
}

// Off-screen standing NPCs still block their tiles. Hidden (masked) map
// objects and wanderers are not projected; loaded ones come from structs.
func TestDecodeGen2LiveObjectsProjectsOffscreenStandingNPCs(t *testing.T) {
	var mem fakeMemory
	put := func(i int, sprite, x, y, move byte, mask byte) {
		base := sym.MapObjects + uint16(i*sym.MapObjectLen)
		mem[base], mem[base+1], mem[base+2], mem[base+3], mem[base+4] = 0xff, sprite, y+4, x+4, move
		mem[sym.ObjectMasks+uint16(i)] = mask
	}
	put(1, 0x2d, 32, 8, 0x08, 0)     // standing left, off screen
	put(2, 0x2d, 14, 10, 0x08, 0xff) // hidden by its event
	put(3, 0x27, 20, 5, 0x02, 0)     // wanderer
	objects, _ := decodeGen2LiveObjects(&mem)
	if len(objects) != 1 || objects[0] != (game.LiveMapObject{Slot: 1, X: 32, Y: 8, Stationary: true}) {
		t.Fatalf("objects = %+v, want only the off-screen standing NPC", objects)
	}
}
