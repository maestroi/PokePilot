package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// TestSilph11FInaccessibleWarpIsWalkThroughFloor: the pret decomp marks
// SILPH_CO_11F (5,5) "; inaccessible". Local walk planning must treat that
// tile as ordinary floor so the president's office stays reachable from the
// 7F pad without routing through the Beauty sprite.
func TestSilph11FInaccessibleWarpIsWalkThroughFloor(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	h, err := rom.ParseMap(romData, silphCo11FMap)
	if err != nil {
		t.Fatal(err)
	}
	if !redInaccessibleWarp(silphCo11FMap, 5, 5) {
		t.Fatal("Silph 11F (5,5) is not classified as an inaccessible warp")
	}
	blocked := warpAvoidance(h, 3, 2, nil)
	if blocked[[2]int{5, 5}] {
		t.Fatal("warpAvoidance banned inaccessible Silph 11F teleporter (5,5)")
	}
	g, err := world.Build(romData, h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := world.FindPath(g, 3, 2, 6, 5, blocked); err != nil {
		t.Fatalf("pad landing (3,2) should reach president approach (6,5) with warpAvoidance: %v", err)
	}
}
