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
