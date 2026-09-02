package world

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestGridBounds(t *testing.T) {
	g := &Grid{Width: 4, Height: 4, walkable: make([]bool, 4*4)}

	if !g.InBounds(0, 0) {
		t.Error("InBounds(0,0) = false, want true")
	}
	if !g.InBounds(3, 3) {
		t.Error("InBounds(3,3) = false, want true")
	}
	if g.InBounds(-1, 0) {
		t.Error("InBounds(-1,0) = true, want false")
	}
	if g.InBounds(4, 0) {
		t.Error("InBounds(4,0) = true, want false")
	}
	if g.InBounds(0, -1) {
		t.Error("InBounds(0,-1) = true, want false")
	}
	if g.InBounds(0, 4) {
		t.Error("InBounds(0,4) = true, want false")
	}

	// Out-of-bounds Walkable must return false, not panic.
	if g.Walkable(-1, 0) {
		t.Error("Walkable(-1,0) = true, want false")
	}
	if g.Walkable(4, 0) {
		t.Error("Walkable(4,0) = true, want false")
	}
	if g.Walkable(0, -1) {
		t.Error("Walkable(0,-1) = true, want false")
	}
	if g.Walkable(0, 4) {
		t.Error("Walkable(0,4) = true, want false")
	}
}

func TestGridSetGet(t *testing.T) {
	g := &Grid{Width: 4, Height: 4, walkable: make([]bool, 4*4)}

	g.Set(2, 3, true)

	if !g.Walkable(2, 3) {
		t.Error("Walkable(2,3) = false after Set(2,3,true), want true")
	}
	if g.Walkable(3, 2) {
		t.Error("Walkable(3,2) = true, want false (x/y transposed)")
	}
	if g.Walkable(0, 0) {
		t.Error("Walkable(0,0) = true, want false")
	}
}

func TestGridTile(t *testing.T) {
	g := &Grid{
		Width: 2, Height: 2,
		walkable: make([]bool, 4),
		tiles:    []uint8{0x10, 0x20, 0x30, 0x3D},
	}
	if got, ok := g.Tile(1, 1); !ok || got != 0x3D {
		t.Fatalf("Tile(1,1) = (%#02x,%v), want (0x3d,true)", got, ok)
	}
	if _, ok := g.Tile(-1, 0); ok {
		t.Fatal("Tile(-1,0) reported an in-bounds value")
	}
	// A hand-built grid that does not carry tile ids must say so rather than
	// returning a misleading zero tile that navigation could classify.
	bare := &Grid{Width: 1, Height: 1, walkable: make([]bool, 1)}
	if _, ok := bare.Tile(0, 0); ok {
		t.Fatal("Tile on a grid without tile ids returned ok=true")
	}
}

func TestBuildPalletTown(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ROM: %v", err)
	}

	const mapID = 0 // Pallet Town
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		t.Fatalf("ParseMap(%d): %v", mapID, err)
	}
	g, err := Build(romData, h)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if want := int(h.WidthBlocks) * 2; g.Width != want {
		t.Errorf("Width = %d, want %d", g.Width, want)
	}
	if want := int(h.HeightBlocks) * 2; g.Height != want {
		t.Errorf("Height = %d, want %d", g.Height, want)
	}
	if g.MapID != mapID {
		t.Errorf("MapID = %d, want %d", g.MapID, mapID)
	}

	walkableCount, solidCount := 0, 0
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			if _, ok := g.Tile(x, y); !ok {
				t.Fatalf("Tile(%d,%d) has no collision id after Build", x, y)
			}
			if g.Walkable(x, y) {
				walkableCount++
			} else {
				solidCount++
			}
		}
	}
	if walkableCount == 0 {
		t.Error("grid has no walkable tiles; collision lookup failed")
	}
	if solidCount == 0 {
		t.Error("grid has no solid tiles; tileset collision list was not actually read")
	}
	t.Logf("Pallet Town grid: %dx%d, %d walkable, %d solid", g.Width, g.Height, walkableCount, solidCount)
}

func TestBuildOaksLabCollision(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ROM: %v", err)
	}

	const mapID = 0x28 // Oak's Lab
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		t.Fatalf("ParseMap(%d): %v", mapID, err)
	}
	g, err := Build(romData, h)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Measured against the game by walking Oak's Lab exhaustively with save
	// states (2026-08-25). The collision tile is the one the player stands
	// on, the step's bottom-left tile, not the tile above it.
	cases := []struct {
		x, y int
		walk bool
	}{
		{0, 2, true}, {1, 2, true}, {2, 2, true}, {3, 2, true}, {4, 2, true},
		{6, 2, true}, {7, 2, true}, {8, 2, true}, {9, 2, true},
		{0, 3, true}, {1, 3, true}, {2, 3, true}, {3, 3, true}, {4, 3, true},
		{5, 3, true}, {9, 3, true},
		{6, 4, true}, {7, 4, true}, {8, 4, true},
		// The table with the three Poke Balls on it.
		{6, 3, false}, {7, 3, false}, {8, 3, false},
		// The top wall.
		{0, 0, false}, {9, 0, false},
	}
	for _, c := range cases {
		if got := g.Walkable(c.x, c.y); got != c.walk {
			t.Errorf("Walkable(%d,%d) = %v, want %v", c.x, c.y, got, c.walk)
		}
	}

	// The regression pair: fails under the old top-left indexing (2*sy),
	// passes under the bottom-left indexing (2*sy+1).
	if !g.Walkable(6, 4) {
		t.Error("Walkable(6,4) = false, want true: collision must use the tile the player stands on (the step's bottom-left), not the tile above it")
	}
	if g.Walkable(6, 3) {
		t.Error("Walkable(6,3) = true, want false: collision must use the tile the player stands on (the step's bottom-left), not the tile above it")
	}
}
