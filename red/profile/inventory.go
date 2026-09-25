package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

func (*Profile) DecodeInventory(reader game.MemoryReader) game.InventoryState {
	if reader == nil {
		return game.InventoryState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	raw := state.DecodeInventory(&mem)
	items := make([]game.InventoryItem, 0, len(raw.Items))
	for _, it := range raw.Items {
		items = append(items, game.InventoryItem{NativeItemID: uint16(it.ID), Quantity: int(it.Quantity)})
	}
	return game.InventoryState{Items: items, Money: raw.Money}
}
