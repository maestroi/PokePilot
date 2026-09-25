package rom

import "fmt"

const (
	npcTradeCount   = 10
	npcTradeNameLen = 11
	npcTradeSize    = 3 + npcTradeNameLen
)

// NPCTrade is one TradeMons row: Index is the stable wWhichTrade value, Give
// is the species the player must select, and Get is the species the NPC
// returns. Dialog and nickname stay ROM-owned.
type NPCTrade struct {
	Index int
	Give  uint8
	Get   uint8
}

// NPCTrades reads the ROM's in-game trade table in table order while
// preserving the original row index. Scripts write that index to wWhichTrade,
// so callers must not renumber rows if a patched/unused row is skipped.
func NPCTrades(romData []byte) ([]NPCTrade, error) {
	off, err := Tables(romData).TradeMons.Offset()
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
		out = append(out, NPCTrade{Index: i, Give: give, Get: get})
	}
	return out, nil
}
