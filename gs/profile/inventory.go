package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

var (
	_ game.InventoryDecoder = (*Profile)(nil)
	_ game.ListMenuDecoder  = (*Profile)(nil)
	_ game.CaptureProfile   = (*Profile)(nil)
)

const (
	gen2ItemUltraBall uint16 = 0x02
	gen2ItemGreatBall uint16 = 0x04
	gen2ItemPokeBall  uint16 = 0x05

	// PACKSTATE_*POCKETMENU in pokegold engine/items/pack.asm: the jumptable
	// state that runs ScrollingMenu for one ordinary pocket.
	gen2PackStateItemsPocketMenu    byte = 2
	gen2PackStateBallsPocketMenu    byte = 4
	gen2PackStateKeyItemsPocketMenu byte = 6
)

// DecodeInventory projects the Items, Balls and Key Items pockets in their
// on-screen order. TM/HM ownership is a separate machine-menu contract.
func (*Profile) DecodeInventory(reader game.MemoryReader) game.InventoryState {
	state := game.InventoryState{}
	if reader == nil {
		return state
	}
	raw := make([]byte, 3)
	reader.PeekInto(sym.Money, raw)
	state.Money = decodeUint24BE(raw)

	appendPairs := func(pocket game.MachinePocket, countAddr, dataAddr uint16, max int) {
		count := min(int(reader.Peek8(countAddr)), max)
		for i := 0; i < count; i++ {
			id := reader.Peek8(dataAddr + uint16(i*2))
			qty := int(reader.Peek8(dataAddr + uint16(i*2+1)))
			if id == 0 || id == 0xff || qty == 0 {
				continue
			}
			state.Items = append(state.Items, game.InventoryItem{NativeItemID: uint16(id), Quantity: qty, Pocket: pocket})
		}
	}
	appendPairs(game.MachinePocketItems, sym.NumItems, sym.Items, sym.MaxItems)
	appendPairs(game.MachinePocketBalls, sym.NumBalls, sym.Balls, sym.MaxBalls)
	keys := min(int(reader.Peek8(sym.NumKeyItems)), sym.MaxKeyItems)
	for i := 0; i < keys; i++ {
		id := reader.Peek8(sym.KeyItems + uint16(i))
		if id == 0 || id == 0xff {
			continue
		}
		state.Items = append(state.Items, game.InventoryItem{NativeItemID: uint16(id), Quantity: 1, Pocket: game.MachinePocketKeyItems})
	}
	return state
}

// DecodeListMenu recognizes the PACK while one ordinary pocket's
// ScrollingMenu is polling. The drawn pack graphic is the liveness proof;
// the jumptable/pocket bytes alone persist after the pack closes.
func (*Profile) DecodeListMenu(reader game.MemoryReader) game.ListMenuState {
	if reader == nil {
		return game.ListMenuState{Position: -1}
	}
	if gsMartItemListLive(reader) {
		return game.ListMenuState{Visible: true, Kind: game.ListMenuItems, Position: gsScrollingMenuPosition(reader)}
	}
	if !gsPackVisible(reader) {
		return game.ListMenuState{Position: -1}
	}
	switch reader.Peek8(sym.JumptableIndex) {
	case gen2PackStateItemsPocketMenu, gen2PackStateBallsPocketMenu, gen2PackStateKeyItemsPocketMenu:
	default:
		return game.ListMenuState{Position: -1}
	}
	if reader.Peek8(sym.MenuCursorY) < 1 {
		return game.ListMenuState{Position: -1}
	}
	return game.ListMenuState{
		Visible:  true,
		Kind:     game.ListMenuItems,
		Position: gsScrollingMenuPosition(reader),
		Pocket:   gsMachinePocket(reader.Peek8(sym.CurPocket)),
	}
}

// gsScrollingMenuPosition is the absolute entry under a live ScrollingMenu
// cursor: the 1-based visible row plus the live scroll offset.
func gsScrollingMenuPosition(reader game.MemoryReader) int {
	return int(reader.Peek8(sym.MenuScrollPosition)) + int(reader.Peek8(sym.MenuCursorY)) - 1
}

// DecodeCapture reports party growth as capture evidence.
// ponytail: party only; add the active SRAM box and wPokedexCaught when a
// full-party catch (which lands in the box) has to be proven.
func (*Profile) DecodeCapture(reader game.MemoryReader) game.CaptureState {
	state := game.CaptureState{}
	if reader == nil {
		return state
	}
	count := min(int(reader.Peek8(sym.PartyCount)), 6)
	for i := 0; i < count; i++ {
		state.PartySpecies = append(state.PartySpecies, uint16(reader.Peek8(sym.PartySpecies+uint16(i))))
	}
	return state
}

// CaptureDexNumber: Gen-II native species ids are national dex numbers.
func (*Profile) CaptureDexNumber(_ []byte, nativeSpecies uint16) (uint16, bool) {
	if nativeSpecies == 0 || nativeSpecies > 251 {
		return 0, false
	}
	return nativeSpecies, true
}

func (*Profile) OrdinaryCaptureBallOrder() []uint16 {
	return []uint16{gen2ItemUltraBall, gen2ItemGreatBall, gen2ItemPokeBall}
}
