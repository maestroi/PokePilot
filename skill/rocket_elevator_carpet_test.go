package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

func TestRocketElevatorFloorEdgesHaveSemanticController(t *testing.T) {
	for _, from := range []uint8{rocketHideoutB1FMap, rocketHideoutB2FMap, rocketHideoutB4FMap} {
		edge := world.Edge{Kind: world.EdgeWarp, From: from, To: rocketHideoutElevatorMap, WarpX: 24, WarpY: 15}
		if !rocketElevatorFloorEdge(edge) {
			t.Fatalf("floor %#02x -> elevator was not classified as carpet warp", from)
		}
		transition, ok := redRouteTransitionForEdge(edge)
		if !ok || transition.ID != "red:rocket_hideout_elevator_carpet" {
			t.Fatalf("floor %#02x -> elevator transition = %+v ok=%v", from, transition, ok)
		}
	}

	reverse := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutElevatorMap, To: rocketHideoutB4FMap, WarpX: 2, WarpY: 1}
	if rocketElevatorFloorEdge(reverse) {
		t.Fatal("elevator -> B4F was incorrectly classified as a floor-side carpet warp")
	}
}

func TestRocketWarpCarpetTableMatchesROMDirections(t *testing.T) {
	tests := []struct {
		push  world.Step
		tile  uint8
		allow bool
	}{
		{world.Step{DX: 0, DY: 1}, 0x3d, true},
		{world.Step{DX: 0, DY: 1}, 0x5c, false},
		{world.Step{DX: 0, DY: -1}, 0x5c, true},
		{world.Step{DX: 0, DY: -1}, 0x3d, false},
		{world.Step{DX: -1, DY: 0}, 0x4b, true},
		{world.Step{DX: -1, DY: 0}, 0x4e, false},
		{world.Step{DX: 1, DY: 0}, 0x4e, true},
		{world.Step{DX: 1, DY: 0}, 0x4b, false},
	}
	for _, tc := range tests {
		if got := rocketWarpCarpetAllows(tc.push, tc.tile); got != tc.allow {
			t.Errorf("push=%v tile=%#02x allow=%v, want %v", tc.push, tc.tile, got, tc.allow)
		}
	}
}

// Issue #577 ended controllable on B4F at (24,15): generic Traverse had
// stepped sideways onto the elevator warp, ExtraWarpCheck rejected the facing
// direction, and holding that same direction for 180 frames could never make
// the map flip. Starting from that exact parked-on-warp coordinate, prove the
// planner can leave the square, choose an approach whose next FieldTile is in
// the ROM's direction-specific carpet list, and make ordinary warpTarget see
// that same entry as a zero-distance push.
func TestRocketB4FElevatorCarpetRecoversFromParkedWarp(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		t.Fatalf("ParseMap(B4F): %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(B4F): %v", err)
	}

	approach, err := planRocketElevatorApproach(h, grid, 24, 15, nil)
	if err != nil {
		t.Fatalf("plan from issue #577 parked warp: %v", err)
	}
	frontX := approach.warpX + approach.push.DX
	frontY := approach.warpY + approach.push.DY
	frontTile, ok := grid.FieldTile(frontX, frontY)
	if !ok {
		t.Fatalf("planned carpet front (%d,%d) is outside B4F", frontX, frontY)
	}
	if !rocketWarpCarpetAllows(approach.push, frontTile) {
		t.Fatalf("planned push %v leaves invalid front tile %#02x at (%d,%d)", approach.push, frontTile, frontX, frontY)
	}

	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  rocketHideoutB4FMap,
		To:    rocketHideoutElevatorMap,
		WarpX: uint8(approach.warpX),
		WarpY: uint8(approach.warpY),
	}
	wx, wy, steps, push, err := warpTarget(h, edge, grid, approach.standX, approach.standY, nil, romData)
	if err != nil {
		t.Fatalf("generic warpTarget from prepared stand: %v", err)
	}
	if wx != approach.warpX || wy != approach.warpY {
		t.Fatalf("warpTarget chose (%d,%d), prepared (%d,%d)", wx, wy, approach.warpX, approach.warpY)
	}
	if len(steps) != 0 {
		t.Fatalf("warpTarget wanted %d extra steps from prepared stand: %v", len(steps), steps)
	}
	if push != approach.push {
		t.Fatalf("warpTarget push=%v, prepared carpet push=%v", push, approach.push)
	}
}
