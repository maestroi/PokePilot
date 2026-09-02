package sym

import "testing"

func TestPlayerDisabledMoveAddressMatchesSymbolFile(t *testing.T) {
	symbols := loadSym(t)
	want, ok := symbols["wPlayerDisabledMove"]
	if !ok {
		t.Fatal("wPlayerDisabledMove not found in vendored symbol file")
	}
	if PlayerDisabledMove != want {
		t.Fatalf("PlayerDisabledMove = 0x%04X, want 0x%04X from vendored symbol file", PlayerDisabledMove, want)
	}
}
