package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

func (*Profile) DecodeInventory(reader game.MemoryReader) game.InventoryState {
	if reader == nil {
		return game.InventoryState{}
	}
	out := game.InventoryState{Items: []game.InventoryItem{}}
	appendPairs := func(countAddr, dataAddr uint16, max int) {
		count := int(reader.Peek8(countAddr))
		if count > max {
			count = max
		}
		for i := 0; i < count; i++ {
			id := reader.Peek8(dataAddr + uint16(i*2))
			qty := int(reader.Peek8(dataAddr + uint16(i*2+1)))
			if id != 0 && qty > 0 {
				out.Items = append(out.Items, game.InventoryItem{NativeItemID: uint16(id), Quantity: qty})
			}
		}
	}
	appendPairs(sym.NumItems, sym.Items, sym.MaxItems)
	appendPairs(sym.NumBalls, sym.Balls, sym.MaxBalls)

	keyCount := int(reader.Peek8(sym.NumKeyItems))
	if keyCount > sym.MaxKeyItems {
		keyCount = sym.MaxKeyItems
	}
	for i := 0; i < keyCount; i++ {
		id := reader.Peek8(sym.KeyItems + uint16(i))
		if id != 0 {
			out.Items = append(out.Items, game.InventoryItem{NativeItemID: uint16(id), Quantity: 1})
		}
	}
	for i, rawID := range gsdata.MachineItems {
		if i >= sym.TMsHMsCount {
			break
		}
		qty := int(reader.Peek8(sym.TMsHMs + uint16(i)))
		if qty > 0 {
			out.Items = append(out.Items, game.InventoryItem{NativeItemID: uint16(rawID), Quantity: qty})
		}
	}
	money := []byte{reader.Peek8(sym.Money), reader.Peek8(sym.Money + 1), reader.Peek8(sym.Money + 2)}
	if n, err := decodeBCD(money); err == nil {
		out.Money = n
	}
	return out
}

var _ game.InventoryDecoder = (*Profile)(nil)
