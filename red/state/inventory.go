package state

import "github.com/maestroi/pokepilot/red/sym"

// BagItem is one bag entry: item ID and quantity.
type BagItem struct {
	ID       uint8
	Quantity uint8
}

// InventoryState is the decoded bag and money.
type InventoryState struct {
	Money uint32 // decoded from BCD
	Items []BagItem
}

// DecodeInventory reads money (3-byte BCD at PlayerMoney) and the bag item
// list. A NumBagItems of 0xFF marks an empty list; counts above 20 (the Gen 1
// bag limit) are clamped.
func DecodeInventory(m *Mem) InventoryState {
	var money uint32
	for _, b := range m.Slice(sym.PlayerMoney, 3) {
		money = money*100 + uint32(b>>4)*10 + uint32(b&0x0F)
	}

	count := int(m.U8(sym.NumBagItems))
	if count == 0xFF {
		count = 0
	}
	if count > 20 {
		count = 20
	}
	items := make([]BagItem, count)
	for n := 0; n < count; n++ {
		off := sym.BagItems + uint16(n)*2
		items[n] = BagItem{ID: m.U8(off), Quantity: m.U8(off + 1)}
	}
	return InventoryState{Money: money, Items: items}
}
