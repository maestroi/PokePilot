package state

import "github.com/maestroi/pokepilot/red/sym"

// BagCapacity is Red's Gen 1 item-pocket size. DecodeInventory clamps to this.
const BagCapacity = 20

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
// list. A NumBagItems of 0xFF marks an empty list; counts above BagCapacity
// are clamped.
func DecodeInventory(m *Mem) InventoryState {
	var money uint32
	for _, b := range m.Slice(sym.PlayerMoney, 3) {
		money = money*100 + uint32(b>>4)*10 + uint32(b&0x0F)
	}

	count := int(m.U8(sym.NumBagItems))
	if count == 0xFF {
		count = 0
	}
	if count > BagCapacity {
		count = BagCapacity
	}
	items := make([]BagItem, count)
	for n := 0; n < count; n++ {
		off := sym.BagItems + uint16(n)*2
		items[n] = BagItem{ID: m.U8(off), Quantity: m.U8(off + 1)}
	}
	return InventoryState{Money: money, Items: items}
}
