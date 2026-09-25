package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	shopWatchActionMenu = 3
	shopWatchListOrQty  = 7
	shopActionMenuMax   = 2
	redViridianMartMap  = 0x2a
)

func (*Profile) DecodeShop(reader game.MemoryReader) game.ShopState {
	if reader == nil {
		return game.ShopState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])

	menu := state.DecodeMenu(&mem)
	text := state.ScreenText(&mem)
	lower := strings.ToLower(text)
	out := game.ShopState{
		Controllable:     state.Controllable(&mem),
		Cursor:           game.MenuCursorState{Current: menu.Current, Max: menu.Max},
		Quantity:         int(mem.U8(sym.ItemQuantity)),
		MaxQuantity:      int(mem.U8(sym.MaxItemQuantity)),
		Total:            decodeShopBCD(&mem),
		Text:             text,
		TradeUnavailable: mem.U8(sym.CurMap) == redViridianMartMap && !state.HasEvent(&mem, state.EventOakGotParcel),
		Unsellable:       strings.Contains(lower, "can't put a") || strings.Contains(lower, "price on that"),
	}
	if out.Controllable {
		out.Phase = game.ShopPhaseClosed
		return out
	}
	if state.DecodeTwoOptionMenu(&mem) != nil {
		out.Phase = game.ShopPhaseConfirmation
		return out
	}
	watched := mem.U8(sym.MenuWatchedKeys)
	switch {
	case watched == shopWatchActionMenu && mem.U8(sym.MaxMenuItem) == shopActionMenuMax:
		out.Phase = game.ShopPhaseActionMenu
	case watched == shopWatchListOrQty && out.MaxQuantity > 0 && out.Quantity >= 1 && out.Quantity <= out.MaxQuantity &&
		(out.MaxQuantity == 99 || strings.Contains(out.Text, "×")):
		out.Phase = game.ShopPhaseQuantity
	case watched == shopWatchListOrQty:
		out.Phase = game.ShopPhaseItemList
	default:
		out.Phase = game.ShopPhaseGreeting
	}
	for i := 1; i < 256; i++ {
		v := mem.U8(sym.ItemList + uint16(i))
		if v == 0xff {
			break
		}
		out.Items = append(out.Items, uint16(v))
	}
	return out
}

func (*Profile) DecodeCenter(reader game.MemoryReader) game.CenterState {
	if reader == nil {
		return game.CenterState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	party := state.DecodeParty(&mem)
	prompt := state.DecodeTwoOptionMenu(&mem) != nil
	menuUp := state.MenuUp(&mem)
	return game.CenterState{
		PartyPresent: party.Count > 0,
		PromptOpen:   prompt,
		Recovered:    partyCenterRecovered(party),
		Controllable: state.Controllable(&mem),
		TextOpen:     mem.U8(sym.FontLoaded) != 0 && !menuUp,
		MenuOpen:     menuUp,
	}
}

func decodeShopBCD(mem *state.Mem) int {
	v := 0
	for _, b := range mem.Slice(sym.Money, 3) {
		v = v*100 + int(b>>4)*10 + int(b&0x0f)
	}
	return v
}
