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

	static, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(B4F): %v", err)
	}
	if _, err := world.FindPath(static, int(rocketB4FEntry.X), int(rocketB4FEntry.Y), int(giovanniStand.X), int(giovanniStand.Y), nil); err == nil {
		t.Fatal("static B4F grid unexpectedly reaches Giovanni through the closed boss door")
	}

	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		t.Fatalf("Blocks(B4F): %v", err)
	}
	liveBlocks := append([]byte(nil), blocks...)
	const doorBlockY, doorBlockX = 5, 12
	idx := doorBlockY*int(h.WidthBlocks) + doorBlockX
	if idx < 0 || idx >= len(liveBlocks) {
		t.Fatalf("B4F door block index %d outside %d blocks", idx, len(liveBlocks))
	}
	// RocketHideoutB4FDoorCallbackScript writes block $0e (Floor block) at
	// bc=(5,12) after both guards are beaten, via ReplaceTileBlock.
	liveBlocks[idx] = 0x0e

	live, err := world.BuildFromBlocks(romData, h, liveBlocks)
	if err != nil {
		t.Fatalf("BuildFromBlocks(B4F live): %v", err)
	}
	if _, err := world.FindPath(live, int(rocketB4FEntry.X), int(rocketB4FEntry.Y), int(giovanniStand.X), int(giovanniStand.Y), nil); err != nil {
		t.Fatalf("live B4F grid cannot reach Giovanni after floor-block replacement: %v", err)
	}
}
