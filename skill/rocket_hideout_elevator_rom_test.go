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

// TestWarpTargetBumpsAWarpTileAlreadyStoodOn is the regression for the stuck
// "go to celadon city" farm runs (triage b7102c933b35d931 and siblings): a
// resumed checkpoint that rode the elevator down and landed exactly on B4F's
// (25,15) — one of the two warp_event tiles the elevator door shares with
// (24,15) — never left. warpTarget used to walk the player sideways onto the
// OTHER door tile and hold the button there, which changed nothing forever:
// pokered only re-checks a tile's door/warp graphic on arrival
// (IsPlayerStandingOnDoorTileOrWarpTile, engine/overworld/doors.asm), and
// neither B4F door tile's collision id (measured 0x42/0x52) is in this
// tileset's door or warp-tile-id lists, so the walking path never fires.
// wMovementFlags' BIT_STANDING_ON_WARP stays set from the original arrival
// until a collision — bumping a wall — fires the warp through
// ExtraWarpCheck/CheckWarpsCollision instead (home/overworld.asm), which is
// exactly the "solid stairs" push mechanic warpTarget already knows for a
// warp tile with no walkable neighbor. Pin that a player already standing on
// one of the destination's warp tiles gets sent into a wall, never onto the
// sibling tile.
func TestWarpTargetBumpsAWarpTileAlreadyStoodOn(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		t.Fatalf("ParseMap(B4F): %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(B4F): %v", err)
	}

	e := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB4FMap, To: rocketHideoutElevatorMap, WarpX: 24, WarpY: 15}
	wx, wy, steps, push, err := warpTarget(h, e, grid, 25, 15, nil, nil, romData)
	if err != nil {
		t.Fatalf("warpTarget already standing on (25,15): %v", err)
	}
	if wx != 25 || wy != 15 {
		t.Fatalf("warpTarget chose (%d,%d), want the tile already stood on (25,15)", wx, wy)
	}
	if len(steps) != 0 {
		t.Fatalf("warpTarget walked %v away from a tile already on the warp; want no walk", steps)
	}
	nx, ny := 25+push.DX, 15+push.DY
	if grid.Walkable(nx, ny) {
		t.Fatalf("warpTarget pushed %s onto walkable (%d,%d); want a wall to bump for the collision-warp path", push, nx, ny)
	}
}

