package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const fakeGen2ShopItem uint16 = 0x0234

type fakeShopPhase uint8

const (
	fakeShopOverworld fakeShopPhase = iota
	fakeShopRoot
	fakeShopList
	fakeShopQuantity
	fakeShopConfirm
	fakeShopText
)

type fakeGen2ShopMachine struct {
	phase                fakeShopPhase
	modeSell             bool
	menuCursor           int
	listPos              int
	quantity             int
	money                int
	itemQty              int
	dropOpeningPresses   int
	overshootOpeningOnce bool
}

func (*fakeGen2ShopMachine) Peek8(uint16) byte       { return 0 }
func (*fakeGen2ShopMachine) PeekInto(uint16, []byte) {}
func (*fakeGen2ShopMachine) StepFrame()              {}
func (m *fakeGen2ShopMachine) StepFrames(n int) {
	for i := 0; i < n; i++ {
		m.StepFrame()
	}
}

func (m *fakeGen2ShopMachine) Tap(btn emu.Button, _, _ int) {
	switch btn {
	case emu.Up:
		if m.phase == fakeShopQuantity {
			m.quantity++
		} else if m.phase == fakeShopRoot && m.menuCursor > 0 {
			m.menuCursor--
		} else if m.phase == fakeShopList && m.listPos > 0 {
			m.listPos--
		}
	case emu.Down:
		if m.phase == fakeShopRoot && m.menuCursor < 2 {
			m.menuCursor++
		} else if m.phase == fakeShopList {
			m.listPos++
		}
	case emu.A:
		switch m.phase {
		case fakeShopOverworld:
			if m.dropOpeningPresses > 0 {
				m.dropOpeningPresses--
				return
			}
			m.phase = fakeShopRoot
			m.menuCursor = 0
			if m.overshootOpeningOnce {
				m.overshootOpeningOnce = false
				m.modeSell = false // default root entry is BUY
				m.phase = fakeShopList
				m.listPos = 0
			}
		case fakeShopRoot:
			m.modeSell = m.menuCursor == 1
			m.phase = fakeShopList
			m.listPos = 0
		case fakeShopList:
			m.phase = fakeShopQuantity
			m.quantity = 1
		case fakeShopQuantity:
			m.phase = fakeShopConfirm
			m.menuCursor = 0
		case fakeShopConfirm:
			if m.modeSell {
				m.itemQty -= m.quantity
				m.money += 25 * m.quantity
			} else {
				m.itemQty += m.quantity
				m.money -= 50 * m.quantity
			}
			m.phase = fakeShopText
		case fakeShopText:
			m.phase = fakeShopList
		}
	case emu.B:
		switch m.phase {
		case fakeShopList, fakeShopQuantity, fakeShopConfirm:
			m.phase = fakeShopRoot
		case fakeShopRoot:
			m.phase = fakeShopOverworld
		}
	}
}

type fakeGen2ShopRuntime struct{}

func (fakeGen2ShopRuntime) DecodeShop(r game.MemoryReader) game.ShopState {
	m := r.(*fakeGen2ShopMachine)
	state := game.ShopState{Quantity: m.quantity, MaxQuantity: 99}
	if m.modeSell {
		state.Total = 25 * m.quantity
	} else {
		state.Total = 50 * m.quantity
	}
	switch m.phase {
	case fakeShopRoot:
		state.Phase = game.ShopPhaseActionMenu
	case fakeShopList:
		state.Phase = game.ShopPhaseItemList
		if m.modeSell {
			state.Items = []uint16{fakeGen2ShopItem}
		} else {
			state.Items = []uint16{0x0101, fakeGen2ShopItem, 0x0345}
		}
	case fakeShopQuantity:
		state.Phase = game.ShopPhaseQuantity
	case fakeShopConfirm:
		state.Phase = game.ShopPhaseConfirmation
	case fakeShopText:
		state.Phase = game.ShopPhaseGreeting
	}
	return state
}

func (fakeGen2ShopRuntime) DecodeInventory(r game.MemoryReader) game.InventoryState {
	m := r.(*fakeGen2ShopMachine)
	items := []game.InventoryItem{}
	if m.itemQty > 0 {
		items = append(items, game.InventoryItem{NativeItemID: fakeGen2ShopItem, Quantity: m.itemQty})
	}
	return game.InventoryState{Items: items, Money: uint32(m.money)}
}

func (fakeGen2ShopRuntime) DecodeOverworld(r game.MemoryReader) game.OverworldState {
	m := r.(*fakeGen2ShopMachine)
	return game.OverworldState{NativeMapID: 0x91, X: 8, Y: 4, Controllable: m.phase == fakeShopOverworld}
}

func (fakeGen2ShopRuntime) DecodeMenuCursor(r game.MemoryReader) game.MenuCursorState {
	m := r.(*fakeGen2ShopMachine)
	max := 2
	if m.phase == fakeShopConfirm {
		max = 1
	}
	return game.MenuCursorState{Current: m.menuCursor, Max: max}
}

func (fakeGen2ShopRuntime) DecodeTwoOption(r game.MemoryReader) (game.TwoOptionState, bool) {
	m := r.(*fakeGen2ShopMachine)
	if m.phase != fakeShopConfirm {
		return game.TwoOptionState{}, false
	}
	return game.TwoOptionState{Current: m.menuCursor}, true
}

func (fakeGen2ShopRuntime) DecodeStartMenu(game.MemoryReader) game.StartMenuState {
	return game.StartMenuState{}
}

func (fakeGen2ShopRuntime) StartMenuEntryIndex(game.MemoryReader, game.StartMenuEntry) (int, bool) {
	return 0, false
}

func (fakeGen2ShopRuntime) DecodeListMenu(r game.MemoryReader) game.ListMenuState {
	m := r.(*fakeGen2ShopMachine)
	return game.ListMenuState{Visible: m.phase == fakeShopList, Kind: game.ListMenuItems, Position: m.listPos}
}

func TestShopTransactionsUseFakeGen2SemanticState(t *testing.T) {
	m := &fakeGen2ShopMachine{money: 1000}
	runtime := fakeGen2ShopRuntime{}

	if err := buyNative(m, runtime, fakeGen2ShopItem, 3); err != nil {
		t.Fatalf("buy fake Gen-II item: %v", err)
	}
	if m.itemQty != 3 || m.money != 850 || m.phase != fakeShopOverworld {
		t.Fatalf("after buy: qty=%d money=%d phase=%d, want 3/850/overworld", m.itemQty, m.money, m.phase)
	}

	if err := sellNative(m, runtime, fakeGen2ShopItem, 2); err != nil {
		t.Fatalf("sell fake Gen-II item: %v", err)
	}
	if m.itemQty != 1 || m.money != 900 || m.phase != fakeShopOverworld {
		t.Fatalf("after sell: qty=%d money=%d phase=%d, want 1/900/overworld", m.itemQty, m.money, m.phase)
	}
}

func TestShopOpenRetriesDroppedClerkInteraction(t *testing.T) {
	m := &fakeGen2ShopMachine{
		money:              1000,
		dropOpeningPresses: 1,
	}
	runtime := fakeGen2ShopRuntime{}

	if err := buyNative(m, runtime, fakeGen2ShopItem, 1); err != nil {
		t.Fatalf("buy after dropped opening A: %v", err)
	}
	if m.itemQty != 1 || m.money != 950 || m.phase != fakeShopOverworld {
		t.Fatalf("after recovered open: qty=%d money=%d phase=%d, want 1/950/overworld", m.itemQty, m.money, m.phase)
	}
}

func TestShopOpenRepairsAccidentalDefaultBuyBeforeSell(t *testing.T) {
	m := &fakeGen2ShopMachine{
		money:                1000,
		itemQty:              3,
		overshootOpeningOnce: true,
	}
	runtime := fakeGen2ShopRuntime{}

	// The opening input jumps straight into the default BUY list. Opening must
	// back out to BUY/SELL/QUIT before sellNative selects SELL, otherwise this
	// would either time out waiting for the root menu or buy the wrong item.
	if err := sellNative(m, runtime, fakeGen2ShopItem, 2); err != nil {
		t.Fatalf("sell after opening overshoot: %v", err)
	}
	if m.itemQty != 1 || m.money != 1050 || m.phase != fakeShopOverworld {
		t.Fatalf("after repaired sell: qty=%d money=%d phase=%d, want 1/1050/overworld", m.itemQty, m.money, m.phase)
	}
}
