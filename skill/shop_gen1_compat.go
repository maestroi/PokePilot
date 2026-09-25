package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// These Gen-I helpers remain for Red-owned PC/item-storage code and legacy
// regression tests. The reusable shop transaction in shop.go does not use
// them; new generations provide game.ShopDecoder instead.
const (
	watchBuySellQuit = 3
	watchListOrQty   = 7
	buySellQuitMax   = 2
)

func buySellQuitUp(mm *state.Mem) bool {
	return mm.U8(sym.FontLoaded) != 0 && mm.U8(sym.MenuWatchedKeys) == watchBuySellQuit &&
		mm.U8(sym.MaxMenuItem) == buySellQuitMax
}

func itemListUp(mm *state.Mem) bool {
	return mm.U8(sym.MenuWatchedKeys) == watchListOrQty
}

func quantityBoxUp(mm *state.Mem, hBefore int, maxBefore uint8) bool {
	if mm.U8(sym.MaxItemQuantity) == 99 && mm.U8(sym.ItemQuantity) >= 1 {
		if maxBefore != 99 || strings.Contains(state.ScreenText(mm), "×") {
			return true
		}
	}
	price := bcdMoney(mm)
	return price > 0 && price != hBefore
}

func twoOptionUp(mm *state.Mem) bool {
	return state.DecodeTwoOptionMenu(mm) != nil
}

func martTimeout(what string, mem *state.Mem) error {
	return fmt.Errorf("%w: skill: Buy: %s did not appear (wFontLoaded=%#04x wCurMenuItem=%d wMaxMenuItem=%d wItemQuantity=%d wMoney=%d)",
		ErrShopMenuTimeout, what, mem.U8(sym.FontLoaded), mem.U8(sym.CurrentMenuItem), mem.U8(sym.MaxMenuItem),
		mem.U8(sym.ItemQuantity), bcdMoney(mem))
}

// selectListEntry is also shared by existing PC/item-storage/interaction code.
// It already delegates cursor semantics to the profile-driven list-menu driver.
func selectListEntry(m *emu.Emu, index int) error {
	if err := selectScrollingListEntry(m, index); err != nil {
		return err
	}
	m.StepFrames(talkSettle)
	return nil
}

// setQuantity is retained for the Gen-I player-PC transaction driver. Shop
// purchases use setShopQuantity and semantic ShopState instead.
func setQuantity(m *emu.Emu, qty int) error {
	for i := 0; i < martQtyBudget; i++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		cur := int(mem.U8(sym.ItemQuantity))
		if cur == qty {
			return nil
		}
		if cur > qty {
			return fmt.Errorf("%w: quantity overshot %d (wItemQuantity=%d)", ErrShopControllerStalled, qty, cur)
		}
		m.Tap(emu.Up, 3, 7)
		m.StepFrames(talkSettle)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	return fmt.Errorf("%w: quantity did not reach %d (wItemQuantity=%d)", ErrShopControllerStalled, qty, mem.U8(sym.ItemQuantity))
}

func martItemPosition(mem *state.Mem, item uint8) (int, bool) {
	for i := 1; ; i++ {
		v := mem.U8(sym.ItemList + uint16(i))
		if v == 0xff {
			break
		}
		if v == item {
			return i - 1, true
		}
	}
	return 0, false
}

func bcdMoney(m *state.Mem) int {
	v := 0
	for _, b := range m.Slice(sym.Money, 3) {
		v = v*100 + int(b>>4)*10 + int(b&0x0f)
	}
	return v
}

func bagCount(items []state.BagItem, id uint8) int {
	for _, it := range items {
		if it.ID == id {
			return int(it.Quantity)
		}
	}
	return 0
}
