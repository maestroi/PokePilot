package rom

import "testing"

func TestNPCTradesReadsGiveGetPairs(t *testing.T) {
	romData := make([]byte, 0x72000)
	off, err := bankedOffset(tradeMonsBank, tradeMonsAddr)
	if err != nil {
		t.Fatal(err)
	}
	// First row: Abra (0x94) for Mr. Mime (0x2A). Remaining rows stay zero
	// and are skipped.
	romData[off] = 0x94
	romData[off+1] = 0x2A

	got, err := NPCTrades(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (NPCTrade{Index: 0, Give: 0x94, Get: 0x2A}) {
		t.Fatalf("trades = %+v, want Abra -> Mr. Mime at index 0", got)
	}
}

func TestNPCTradesPreservesIndexAcrossSkippedRows(t *testing.T) {
	romData := make([]byte, 0x72000)
	off, err := bankedOffset(tradeMonsBank, tradeMonsAddr)
	if err != nil {
		t.Fatal(err)
	}
	// Leave row 0 empty and put a valid trade in row 1. Scripts address
	// TradeMons by the original row index, so the parser must not renumber it.
	row := off + npcTradeSize
	romData[row] = 0x94
	romData[row+1] = 0x2A

	got, err := NPCTrades(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Index != 1 {
		t.Fatalf("trades = %+v, want preserved index 1", got)
	}
}

func TestNPCTradesFollowsPatchedTable(t *testing.T) {
	romData := make([]byte, 0x72000)
	off, err := bankedOffset(tradeMonsBank, tradeMonsAddr)
	if err != nil {
		t.Fatal(err)
	}
	romData[off] = 0x05   // Spearow
	romData[off+1] = 0x40 // Farfetch'd

	got, err := NPCTrades(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Index != 0 || got[0].Get != 0x40 {
		t.Fatalf("patched trade = %+v, want Farfetch'd at index 0", got)
	}
}

func TestNPCTradesFromROM(t *testing.T) {
	romData := loadROM(t)
	got, err := NPCTrades(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != npcTradeCount {
		t.Fatalf("len(trades) = %d, want %d", len(got), npcTradeCount)
	}
	if !hasTrade(got, 0x94, 0x2A) { // Abra -> Mr. Mime
		t.Fatalf("missing Abra/Mr. Mime trade: %+v", got)
	}
}

func hasTrade(trades []NPCTrade, give, get uint8) bool {
	for _, tr := range trades {
		if tr.Give == give && tr.Get == get {
			return true
		}
	}
	return false
}
