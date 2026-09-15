package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// Issue #551 exposed a story-topology fact that the old progression sequence
// had backwards: the B3F stair enters B4F on the Lift Key side, while the two
// boss-door guards are reachable only from the elevator side until they have
// both been defeated. Pin that distinction against the real ROM so a future
// route cannot regress to "stairs -> guards" again.
func TestRocketB4FLockedDoorRequiresElevatorForGuards(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		t.Fatalf("ParseMap(B4F): %v", err)
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		t.Fatalf("Blocks(B4F): %v", err)
	}
	blocks = append([]byte(nil), blocks...)
	idx := 5*int(h.WidthBlocks) + 12
	if idx < 0 || idx >= len(blocks) {
		t.Fatalf("boss-door block index %d outside %d blocks", idx, len(blocks))
	}
	blocks[idx] = 0x2d // RocketHideoutB4FDoorCallbackScript's locked block.
	grid, err := world.BuildFromBlocks(romData, h, blocks)
	if err != nil {
		t.Fatalf("BuildFromBlocks(B4F locked): %v", err)
	}

	if _, _, err := world.FindPathAdjacent(grid, int(rocketB4FEntry.X), int(rocketB4FEntry.Y), int(rocketGuard1X), int(rocketGuard1Y), nil); !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("stairs -> guard 1 with locked door err=%v, want world.ErrNoPath", err)
	}
	if _, _, err := world.FindPathAdjacent(grid, int(rocketB4FEntry.X), int(rocketB4FEntry.Y), int(rocketLiftKeyRocketX), int(rocketLiftKeyRocketY), nil); err != nil {
		t.Fatalf("stairs -> Lift Key Rocket should be reachable before elevator: %v", err)
	}
	if _, _, err := world.FindPathAdjacent(grid, 25, 14, int(rocketGuard1X), int(rocketGuard1Y), nil); err != nil {
		t.Fatalf("elevator side -> guard 1 should be reachable with door locked: %v", err)
	}
}

func TestRocketHideoutReverseSpinnerRoutesToB2F(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, rocketHideoutB3FMap)
	if err != nil {
		t.Fatalf("ParseMap(B3F): %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(B3F): %v", err)
	}
	actions, err := planRocketSpinner(grid.Width, grid.Height, grid.Walkable, 19, 17, 25, 6, rocketB3FSpins, nil)
	if err != nil {
		t.Fatalf("reverse B3F spinner plan: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("reverse B3F spinner plan was empty away from B2F warp")
	}
}

// Issue #570 failed after the Lift Key route unnecessarily climbed from B2F
// to B1F and then planned through B1F's runtime-replaced door using immutable
// ROM collision. B2F already owns an elevator entrance. Prove that the
// spinner-aware planner can reach it directly from the B3F stair arrival area.
func TestRocketHideoutB2FSpinnerRoutesToElevator(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, rocketHideoutB2FMap)
	if err != nil {
		t.Fatalf("ParseMap(B2F): %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(B2F): %v", err)
	}

	const (
		startX = 21
		startY = 9
		warpX  = 24
		warpY  = 19
	)
	actions, err := planRocketSpinner(grid.Width, grid.Height, grid.Walkable, startX, startY, warpX, warpY, rocketB2FSpins, nil)
	if err != nil {
		t.Fatalf("B2F elevator spinner plan: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("B2F elevator spinner plan was empty away from elevator warp")
	}

	at := rocketPoint{startX, startY}
	for _, action := range actions {
		at = action.Landing
	}
	dx := at.x - warpX
	if dx < 0 {
		dx = -dx
	}
	dy := at.y - warpY
	if dy < 0 {
		dy = -dy
	}
	if dx+dy != 1 {
		t.Fatalf("B2F elevator spinner plan ended at (%d,%d), not beside warp (%d,%d)", at.x, at.y, warpX, warpY)
	}
}
