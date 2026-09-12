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
	if len(got) != 1 || got[0] != (NPCTrade{Give: 0x94, Get: 0x2A}) {
		t.Fatalf("trades = %+v, want Abra -> Mr. Mime", got)
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
	if len(got) != 1 || got[0].Get != 0x40 {
		t.Fatalf("patched trade = %+v, want Farfetch'd", got)
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
