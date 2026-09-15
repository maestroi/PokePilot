package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

func TestLiveBlockGridRoutesThroughRocketBossDoor(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		t.Fatalf("ParseMap(B4F): %v", err)
	}

	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		t.Fatalf("Blocks(B4F): %v", err)
	}
	const doorBlockY, doorBlockX = 5, 12
	idx := doorBlockY*int(h.WidthBlocks) + doorBlockX
	if idx < 0 || idx >= len(blocks) {
		t.Fatalf("B4F door block index %d outside %d blocks", idx, len(blocks))
	}

	// rocketB4FEntry (the west/stair side) is not walkably connected to the
	// guards' side at all, door or no door (RocketHideout's own comment on
	// reachRocketGuardSide); the only foot route to Giovanni starts from the
	// elevator's B4F landing tile (25,15), one of its own def_warp_events.
	// giovanniStand (25,4) is the desk tile in front of Giovanni, not itself
	// walkable (MEASURED: only (25,3), Giovanni's own tile, borders it), so
	// route to a tile adjacent to it, the same as walkRocketBossDoor does.
	const elevatorLandingX, elevatorLandingY = 25, 15

	// RocketHideoutB4FDoorCallbackScript writes block $2d (door block) at
	// bc=(5,12) while the two guards are unbeaten. This ROM's compiled B4F
	// map already ships that cell as $0e (MEASURED), so build the closed
	// state explicitly instead of assuming world.Build's raw bytes reflect
	// it.
	closedBlocks := append([]byte(nil), blocks...)
	closedBlocks[idx] = 0x2d
	closed, err := world.BuildFromBlocks(romData, h, closedBlocks)
	if err != nil {
		t.Fatalf("BuildFromBlocks(B4F closed): %v", err)
	}
	if _, _, err := world.FindPathAdjacent(closed, elevatorLandingX, elevatorLandingY, int(giovanniStand.X), int(giovanniStand.Y), nil); err == nil {
		t.Fatal("closed-door B4F grid unexpectedly reaches Giovanni through the boss door")
	}

	// RocketHideoutB4FDoorCallbackScript writes block $0e (Floor block) at
	// bc=(5,12) once both guards are beaten, via ReplaceTileBlock.
	liveBlocks := append([]byte(nil), blocks...)
	liveBlocks[idx] = 0x0e
	live, err := world.BuildFromBlocks(romData, h, liveBlocks)
	if err != nil {
		t.Fatalf("BuildFromBlocks(B4F live): %v", err)
	}
	if _, _, err := world.FindPathAdjacent(live, elevatorLandingX, elevatorLandingY, int(giovanniStand.X), int(giovanniStand.Y), nil); err != nil {
		t.Fatalf("live B4F grid cannot reach Giovanni after floor-block replacement: %v", err)
	}
}
