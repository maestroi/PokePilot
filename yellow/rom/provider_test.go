package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

// The pair/ledge tables are read from Yellow's own ROM addresses; the values
// asserted here come from pokeyellow/data/tilesets/pair_collision_tile_ids.asm
// and ledge_tiles.asm.
func TestRealYellowCollisionTablesMatchDecomp(t *testing.T) {
	data := loadRealYellowROM(t)
	land := yellowTilePairsForTraversal(data, 17, worldmodel.TraversalLand)
	if !land[[2]uint8{0x20, 0x05}] || !land[[2]uint8{0x05, 0x20}] {
		t.Fatal("Cavern land pair 20/05 missing or not symmetric")
	}
	water := yellowTilePairsForTraversal(data, 3, worldmodel.TraversalWater)
	if !water[[2]uint8{0x14, 0x2e}] {
		t.Fatal("Forest water pair 14/2e missing")
	}
	if got := yellowLedges(data, 0); len(got) != 8 {
		t.Fatalf("overworld ledges = %d, want 8", len(got))
	}
	if got := yellowLedges(data, 24); len(got) != 0 {
		t.Fatalf("Beach House unexpectedly has overworld ledges: %+v", got)
	}
}

func TestYellowElevatorTopology(t *testing.T) {
	spec, ok := lookupElevator(0xEC)
	if !ok || len(spec.Floors) != 11 {
		t.Fatalf("Silph elevator = %+v, %v", spec, ok)
	}
	floor, ok := elevatorFloorForDestination(0xEC, 0xEB)
	if !ok || floor.DestWarpID != 1 {
		t.Fatalf("11F destination = %+v, %v", floor, ok)
	}
}
