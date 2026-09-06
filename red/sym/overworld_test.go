package sym

import "testing"

func TestOverworldMapAddressMatchesSymbolFile(t *testing.T) {
	symbols := loadSym(t)
	start, ok := symbols["wOverworldMap"]
	if !ok {
		t.Fatal("wOverworldMap not found in vendored symbol file")
	}
	end, ok := symbols["wOverworldMapEnd"]
	if !ok {
		t.Fatal("wOverworldMapEnd not found in vendored symbol file")
	}
	if start != OverworldMap {
		t.Fatalf("OverworldMap = %#04x, want %#04x", OverworldMap, start)
	}
	if got := int(end - start); got != OverworldMapLen {
		t.Fatalf("OverworldMapLen = %d, want %d from symbol span", OverworldMapLen, got)
	}
}
