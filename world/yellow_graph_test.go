package world

import (
	"os"
	"testing"

	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

// Smoke-test the Yellow map-level graph with Yellow's own tables. The planner's
// Travel layer depends on this graph existing and being internally consistent:
// every warp edge must land on a real node, and PalletTown must reach its
// neighbours.
func TestYellowGraphBuilds(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	g, err := BuildGraphForTables(yellowrom.Tables(), romData)
	if err != nil {
		t.Fatal(err)
	}
	n := len(g.Edges)
	if n < 200 {
		t.Errorf("graph has %d nodes, want >= 200", n)
	}
	t.Logf("Yellow graph: %d map nodes", n)

	// SUMMER_BEACH_HOUSE ($F8) must be a node: Red's cutoff would drop it.
	if _, ok := g.Edges[0xF8]; !ok {
		t.Error("SUMMER_BEACH_HOUSE (0xF8) is not a node; Yellow's tail was cut")
	}
	if _, ok := g.Edges[0xF7]; !ok {
		t.Error("AGATHAS_ROOM (0xF7) is not a node")
	}

	// No edge may point at a node that does not exist (except the resolved
	// 0xFF "came from" case, which BuildGraph resolves or drops by contract).
	for from, edges := range g.Edges {
		for _, e := range edges {
			if _, ok := g.Edges[e.To]; !ok {
				t.Errorf("edge from 0x%02x points at missing node 0x%02x", from, e.To)
			}
		}
	}

	// PalletTown must reach somewhere: warps to REDS_HOUSE_1F/BLUES_HOUSE/OAKS_LAB
	// plus its two connections.
	if edges := g.Edges[0x00]; len(edges) == 0 {
		t.Error("PalletTown (0x00) has no edges")
	} else {
		t.Logf("PalletTown has %d edges", len(edges))
	}
}
