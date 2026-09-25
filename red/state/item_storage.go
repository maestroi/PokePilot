package state

import "github.com/maestroi/pokepilot/red/sym"

// PCItemStorageState is the Player PC's item-storage inventory. Gen I keeps
// this separate from Bill's Pokemon boxes: up to 50 distinct item stacks can
// be stored here and later withdrawn through MY PC / ITEM STORAGE SYSTEM.
type PCItemStorageState struct {
	Items []BagItem
}

// DecodePCItemStorage reads wNumBoxItems/wBoxItems from a RAM snapshot. A
// corrupt count is clamped to the ROM's fixed PC_ITEM_CAPACITY, matching the
// defensive behavior of the bag/party decoders.
func DecodePCItemStorage(m *Mem) PCItemStorageState {
	count := int(m.U8(sym.PCItemCount))
	if count > sym.PCItemCapacity {
		count = sym.PCItemCapacity
	}
	items := make([]BagItem, 0, count)
	for i := 0; i < count; i++ {
		base := sym.PCItems + uint16(i*2)
		id := m.U8(base)
		qty := m.U8(base + 1)
		if id == 0 || id == 0xff {
			break
		}
		items = append(items, BagItem{ID: id, Quantity: qty})
	}
	return PCItemStorageState{Items: items}
}
