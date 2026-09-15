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

func TestRocketHideoutReverseSpinnerRoutesToB1F(t *testing.T) {
	romData := rocketHideoutROM(t)

	for _, tc := range []struct {
		name       string
		mapID      uint8
		sx, sy     int
		warpX      int
		warpY      int
		transitions map[rocketPoint]rocketPoint
	}{
		{
			name: "B3F back to B2F",
			mapID: rocketHideoutB3FMap,
			sx: 19, sy: 17,
			warpX: 25, warpY: 6,
			transitions: rocketB3FSpins,
		},
		{
			name: "B2F back to B1F",
			mapID: rocketHideoutB2FMap,
			sx: 21, sy: 9,
			warpX: 27, warpY: 8,
			transitions: rocketB2FSpins,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := rom.ParseMap(romData, tc.mapID)
			if err != nil {
				t.Fatalf("ParseMap(%02x): %v", tc.mapID, err)
			}
			grid, err := world.Build(romData, h)
			if err != nil {
				t.Fatalf("Build(%02x): %v", tc.mapID, err)
			}
			actions, err := planRocketSpinner(grid.Width, grid.Height, grid.Walkable, tc.sx, tc.sy, tc.warpX, tc.warpY, tc.transitions, nil)
			if err != nil {
				t.Fatalf("reverse spinner plan: %v", err)
			}
			if len(actions) == 0 {
				t.Fatal("reverse spinner plan was empty away from target warp")
			}
		})
	}
}
