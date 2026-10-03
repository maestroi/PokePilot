package skill

import (
	"os"
	"testing"
)

// TestCatchHabitatDestination pins the invariant a wild-grass catch relies on:
// a broad habitat name resolves to a map-arrival goal, which is a no-op when the
// player is already on the map but in a walkable component with no encounter
// cells. Route 10 is the measured case (run-2jhhlxdx9kag93jojo2wku7pj1): its
// grass sits in the north component while the Route 9 seam lands the player in
// the south component, which reaches the grass only through the Rock Tunnel.
// Left as a map goal, Travel never moves and Catch reports "no encounter cells
// reachable". CatchHabitatDestination must refine such a goal to the habitat's
// canonical tile when that tile is inside the encounter-cell component.
func TestCatchHabitatDestination(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read POKEMON_RED_ROM: %v", err)
	}

	// Route 10: the canonical tile (11,20) is in the grass component, so a map
	// goal must refine to that exact tile.
	d, ok := Place("route 10")
	if !ok {
		t.Fatalf("Place(route 10) not found")
	}
	if d.Kind != DestinationMap {
		t.Fatalf("Place(route 10) kind=%v, want DestinationMap precondition", d.Kind)
	}
	got := CatchHabitatDestination(romData, d)
	if got.Kind != DestinationExactTile {
		t.Fatalf("CatchHabitatDestination(route 10) kind=%v, want DestinationExactTile", got.Kind)
	}
	if got.Map != 0x15 || got.X != 11 || got.Y != 20 {
		t.Fatalf("CatchHabitatDestination(route 10) = (%#04x,%d,%d), want (0x15,11,20)", got.Map, got.X, got.Y)
	}

	// Route 4: the canonical tile (10,10) is NOT in the encounter-cell component
	// (the map is fragmented), so the goal must keep its map-arrival semantics
	// rather than be pointed at an off-grass tile.
	d4, ok := Place("route 4")
	if !ok {
		t.Fatalf("Place(route 4) not found")
	}
	if got := CatchHabitatDestination(romData, d4); got.Kind != DestinationMap {
		t.Fatalf("CatchHabitatDestination(route 4) kind=%v, want DestinationMap (off-grass hint unchanged)", got.Kind)
	}

	// An exact-tile destination is never altered.
	exact := ExactDestination(0x15, 6, 71)
	if got := CatchHabitatDestination(romData, exact); got.Kind != DestinationExactTile || got.X != 6 || got.Y != 71 {
		t.Fatalf("CatchHabitatDestination altered an exact destination: %+v", got)
	}
}
