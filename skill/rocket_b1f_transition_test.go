package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// TestRocketB1FTrainerDoorOnlyRunsFromLockedSide is the regression for #895.
// #587 correctly taught GoTo to fight Rocket5 when the elevator lands south of
// B1F's runtime door, but the semantic action was also executed for checkpoints
// already north of that door. From the north side, the Game Corner exit is
// directly reachable and Rocket5 is not; forcing the fight therefore turned a
// valid escape to Celadon into world.ErrNoPath.
func TestRocketB1FTrainerDoorOnlyRunsFromLockedSide(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, rocketHideoutB1FMap)
	if err != nil {
		t.Fatalf("ParseMap(B1F): %v", err)
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		t.Fatalf("Blocks(B1F): %v", err)
	}
	blocks = append([]byte(nil), blocks...)

	// RocketHideoutB1FDoorCallbackScript writes block $54 at block (12,8)
	// until EVENT_BEAT_ROCKET_HIDEOUT_1_TRAINER_4 is set.
	idx := 8*int(h.WidthBlocks) + 12
	if idx < 0 || idx >= len(blocks) {
		t.Fatalf("B1F door block index %d outside %d blocks", idx, len(blocks))
	}
	blocks[idx] = 0x54
	grid, err := world.BuildFromBlocks(romData, h, blocks)
	if err != nil {
		t.Fatalf("BuildFromBlocks(B1F locked): %v", err)
	}

	exit := world.Edge{
		Kind:  world.EdgeWarp,
		From:  rocketHideoutB1FMap,
		To:    gameCornerMap,
		WarpX: rocketB1FGameCornerWarpX,
		WarpY: rocketB1FGameCornerWarpY,
	}

	for _, tc := range []struct {
		name      string
		x, y      int
		reachable bool
	}{
		{name: "north exit side", x: 21, y: 3, reachable: true},
		{name: "south elevator side", x: 25, y: 19, reachable: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rocketB1FExitReachableOnGrid(h, exit, grid, tc.x, tc.y, romData)
			if err != nil {
				t.Fatalf("rocketB1FExitReachableOnGrid from (%d,%d): %v", tc.x, tc.y, err)
			}
			if got != tc.reachable {
				t.Fatalf("exit reachable from (%d,%d) = %v, want %v", tc.x, tc.y, got, tc.reachable)
			}
		})
	}
}
