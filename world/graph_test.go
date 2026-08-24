package world

import (
	"os"
	"testing"
)

func loadGraph(t *testing.T) *Graph {
	t.Helper()
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ROM: %v", err)
	}
	g, err := BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	return g
}

func hasWarpEdge(g *Graph, from, to, x, y uint8) bool {
	for _, e := range g.Edges[from] {
		if e.Kind == EdgeWarp && e.To == to && e.WarpX == x && e.WarpY == y {
			return true
		}
	}
	return false
}

func hasConnEdge(g *Graph, from, to, dir uint8) bool {
	for _, e := range g.Edges[from] {
		if e.Kind == EdgeConnection && e.To == to && e.Dir == dir {
			return true
		}
	}
	return false
}

func TestBuildGraphCoversMaps(t *testing.T) {
	g := loadGraph(t)
	if n := len(g.Edges); n < 200 {
		t.Errorf("graph covers %d maps, want at least 200", n)
	}
}

func TestBuildGraphNoUnresolvedDestinations(t *testing.T) {
	g := loadGraph(t)
	for from, edges := range g.Edges {
		for _, e := range edges {
			if e.To == 0xFF {
				t.Errorf("edge %02X->%02X (kind %d) has unresolved To 0xFF", from, e.To, e.Kind)
			}
		}
	}
}

func TestBuildGraphRedsHouse(t *testing.T) {
	g := loadGraph(t)

	// Red's House 2F (0x26) warps down to 1F (0x25) at (7,1).
	if !hasWarpEdge(g, 0x26, 0x25, 7, 1) {
		t.Error("map 0x26 missing warp edge to 0x25 at (7,1)")
	}
	// Red's House 1F (0x25) warps up to 2F (0x26) at (7,1).
	if !hasWarpEdge(g, 0x25, 0x26, 7, 1) {
		t.Error("map 0x25 missing warp edge to 0x26 at (7,1)")
	}
	// 1F's exit warps have DestMap 0xFF ("the map you came from") and must
	// resolve to Pallet Town (0x00).
	if !hasWarpEdge(g, 0x25, 0x00, 2, 7) {
		t.Error("map 0x25 missing 0xFF warp edge to 0x00 at (2,7)")
	}
	if !hasWarpEdge(g, 0x25, 0x00, 3, 7) {
		t.Error("map 0x25 missing 0xFF warp edge to 0x00 at (3,7)")
	}
}

func TestBuildGraphConnectionsAndCities(t *testing.T) {
	g := loadGraph(t)

	// Pallet Town (0x00) connects north to Route 1 (0x0C).
	if !hasConnEdge(g, 0x00, 0x0C, 0) {
		t.Error("map 0x00 missing connection edge to 0x0C with Dir 0 (north)")
	}
	// Viridian City (0x01) warps to 0x29 at (23,25).
	if !hasWarpEdge(g, 0x01, 0x29, 23, 25) {
		t.Error("map 0x01 missing warp edge to 0x29 at (23,25)")
	}
}
