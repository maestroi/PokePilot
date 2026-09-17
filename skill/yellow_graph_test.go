package skill

import (
	"os"
	"testing"
)

// Travel routes over cachedRouteGraph. On a Yellow ROM it must use Yellow's
// map tables: Red's parse only 33 of Yellow's 249 maps, so a Yellow route
// computed under Red's tables is missing most of the world. This pins the
// resolver choice and the node count it produces.
func TestCachedRouteGraphYellow(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	if got := graphForROM(romData); got.IsZero() {
		t.Error("graphForROM returned an unconstructed Tables on the Yellow ROM")
	}
	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(g.Edges); n < 200 {
		t.Errorf("Yellow travel graph has %d nodes; Red tables were used", n)
	} else {
		t.Logf("Yellow travel graph: %d nodes", n)
	}
	if _, ok := g.Edges[0xF8]; !ok {
		t.Error("SUMMER_BEACH_HOUSE (0xF8) missing from the travel graph")
	}
}
