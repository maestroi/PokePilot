package rom

import "fmt"

const (
	tradeMonsBank   uint8  = 0x1C
	tradeMonsAddr   uint16 = 0x5B7B
	npcTradeCount          = 10
	npcTradeNameLen        = 11
	npcTradeSize           = 3 + npcTradeNameLen
)

// NPCTrade is one TradeMons row: the species the player must give and the
// species the NPC returns. Dialog and nickname stay in the ROM; Dex mode
// only needs the species pair.
type NPCTrade struct {
	Give uint8
	Get  uint8
}

// NPCTrades reads the ROM's in-game trade table in table order.
func NPCTrades(romData []byte) ([]NPCTrade, error) {
	off, err := bankedOffset(tradeMonsBank, tradeMonsAddr)
	if err != nil {
		return nil, fmt.Errorf("rom: TradeMons: %w", err)
	}
	if off+npcTradeCount*npcTradeSize > len(romData) {
		return nil, fmt.Errorf("rom: TradeMons at %#x exceeds ROM of %d bytes", off, len(romData))
	}
	out := make([]NPCTrade, 0, npcTradeCount)
	for i := 0; i < npcTradeCount; i++ {
		entry := off + i*npcTradeSize
		give := romData[entry]
		get := romData[entry+1]
		if give == 0 || get == 0 {
			continue
		}
		out = append(out, NPCTrade{Give: give, Get: get})
	}
	return out, nil
}
