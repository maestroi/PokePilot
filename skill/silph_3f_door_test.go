package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// TestSilphCo3FCorridorDoorGatesRivalApproach documents the live topology
// that ClearSilphCo must respect: with both Card Key doors closed, the stair
// landing cannot reach the rival-room door's approach tiles. Opening only the
// mid-corridor door at block (8,4) restores that approach; the rival-room
// door at (4,4) is a second, later unlock.
func TestSilphCo3FCorridorDoorGatesRivalApproach(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	h, err := rom.ParseMap(romData, silphCo3FMap)
	if err != nil {
		t.Fatal(err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatal(err)
	}
	// Closed-door geometry from SilphCo3FGateCallbackScript's $5f replacements.
	for _, p := range [][2]int{{9, 8}, {9, 9}, {17, 8}, {17, 9}} {
		grid.Set(p[0], p[1], false)
	}

	sx, sy := int(silph3FStairLandingX), int(silph3FStairLandingY)
	ax, ay := int(silph3FCorridorDoorApproachX), int(silph3FCorridorDoorApproachY)
	if _, err := world.FindPath(grid, sx, sy, ax, ay, nil); err == nil {
		t.Fatalf("stair landing reached corridor approach with both doors closed")
	}

	// Opening corridor door block (8,4) restores (17,8)/(17,9).
	for _, p := range [][2]int{{17, 8}, {17, 9}} {
		grid.Set(p[0], p[1], true)
	}
	if _, err := world.FindPath(grid, sx, sy, ax, ay, nil); err != nil {
		t.Fatalf("after corridor door open: stair landing -> approach: %v", err)
	}
}

func TestSilphCo3FDoorConstantsMatchDecomp(t *testing.T) {
	if silph3FCorridorDoorBlockX != 8 || silph3FCorridorDoorBlockY != 4 {
		t.Fatalf("corridor door block = (%d,%d), want (8,4) from SilphCo3F.asm GateCoordinates",
			silph3FCorridorDoorBlockX, silph3FCorridorDoorBlockY)
	}
	if silph3FDoorBlockX != 4 || silph3FDoorBlockY != 4 {
		t.Fatalf("rival-room door block = (%d,%d), want (4,4)", silph3FDoorBlockX, silph3FDoorBlockY)
	}
	cells := replacedBlockCells(silph3FCorridorDoorBlockY, silph3FCorridorDoorBlockX)
	want := map[[2]int]bool{{16, 8}: true, {17, 8}: true, {16, 9}: true, {17, 9}: true}
	for _, c := range cells {
		if !want[c] {
			t.Fatalf("corridor door cells = %v, unexpected %v", cells, c)
		}
	}
}
