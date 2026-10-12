package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

var _ game.ShopDecoder = (*Profile)(nil)

// STANDARDMART_* in pokegold constants/mart_constants.asm.
const (
	gen2MartHowMayIHelp byte = iota
	gen2MartTopMenu
	gen2MartBuy
	gen2MartSell
	gen2MartQuit
	gen2MartAnythingElse
)

// gen2MartListRows is MenuHeader_Buy's ScrollingMenu height.
const gen2MartListRows = 4

// gsMartItemListLive recognizes BuyMenuLoop's ScrollingMenu. The jumptable
// byte survives the conversation, so the 4-row, single-column cursor shape
// and the absence of the quantity/confirmation boxes are the liveness proof.
func gsMartItemListLive(reader game.MemoryReader) bool {
	if reader.Peek8(sym.MartJumptableIndex) != gen2MartBuy || gsControllable(reader) {
		return false
	}
	_, _, rows, cols, _ := gsMenuCursor(reader)
	if rows != gen2MartListRows || cols != 1 {
		return false
	}
	text := gsScreenText(reader)
	return !strings.Contains(text, "How many?") && !strings.Contains(text, "YES")
}

// DecodeShop projects a standard Poke Mart conversation (pokegold
// engine/items/mart.asm StandardMart). Text boxes that wait for A — welcome,
// "Here you are", "anything else", "come again" — are the greeting phase;
// blank redraws between boxes are busy.
func (p *Profile) DecodeShop(reader game.MemoryReader) game.ShopState {
	state := game.ShopState{}
	if reader == nil {
		return state
	}
	state.Controllable = gsControllable(reader)
	if state.Controllable {
		return state
	}
	state.Text = gsScreenText(reader)
	state.Cursor = p.DecodeMenuCursor(reader)
	count := min(int(reader.Peek8(sym.CurMartCount)), sym.MaxMartItems)
	for i := 0; i < count; i++ {
		id := reader.Peek8(sym.CurMartItems + uint16(i))
		if id == 0xff {
			break
		}
		state.Items = append(state.Items, uint16(id))
	}

	switch reader.Peek8(sym.MartJumptableIndex) {
	case gen2MartTopMenu:
		_, _, rows, cols, _ := gsMenuCursor(reader)
		if rows == 3 && cols == 1 && strings.Contains(state.Text, "BUY") && strings.Contains(state.Text, "QUIT") {
			state.Phase = game.ShopPhaseActionMenu
		} else {
			state.Phase = game.ShopPhaseBusy
		}
	case gen2MartBuy:
		_, confirm := p.DecodeTwoOption(reader)
		switch {
		case confirm:
			state.Phase = game.ShopPhaseConfirmation
		case strings.Contains(state.Text, "How many?"):
			state.Phase = game.ShopPhaseQuantity
			state.Quantity = int(reader.Peek8(sym.ItemQuantityChange))
			state.MaxQuantity = 99
			raw := make([]byte, 3)
			reader.PeekInto(sym.MoneyTemp, raw)
			state.Total = int(decodeUint24BE(raw))
		case gsMartItemListLive(reader):
			state.Phase = game.ShopPhaseItemList
		case gsMartWaitText(state.Text):
			state.Phase = game.ShopPhaseGreeting
		default:
			state.Phase = game.ShopPhaseBusy
		}
	case gen2MartHowMayIHelp, gen2MartQuit, gen2MartAnythingElse:
		state.Phase = game.ShopPhaseBusy
		if state.Text != "" {
			state.Phase = game.ShopPhaseGreeting
		}
	}
	return state
}

// gsMartWaitText names the purchase outcome boxes BuyMenuLoop closes with
// JoyWaitAorB (pokegold data/text/common_2.asm). Anything else inside BUY is
// the list redrawing, where an A press would select an item.
func gsMartWaitText(text string) bool {
	for _, want := range []string{"Here you are.", "enough money.", "any more items."} {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}
