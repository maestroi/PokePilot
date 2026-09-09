package world

import "testing"

func TestCeruleanTradeHouseReturnsToRealOverworld(t *testing.T) {
	g := loadGraph(t)
	if _, ok := g.Edges[0x0b]; ok {
		t.Error("unused map 0b admitted as routing node")
	}
	exits := g.Edges[0x3f]
	if len(exits) != 2 {
		t.Fatalf("trade house exits = %v, want both door tiles", exits)
	}
	for _, e := range exits {
		if e.To != 0x03 {
			t.Errorf("trade house exit = %+v, want Cerulean City", e)
		}
	}
}
