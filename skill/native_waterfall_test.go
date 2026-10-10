package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

func waterfallTestGrid(t *testing.T, mode worldmodel.TraversalMode) *world.NativeGrid {
	t.Helper()
	spec := worldmodel.NativeGridSpec{
		MapID:         0x0204,
		Width:         3,
		Height:        4,
		Walkable:      make([]bool, 12),
		CollisionTile: make([]byte, 12),
		Obstacles:     make([]worldmodel.NativeObstacle, 12),
		Traversal:     mode,
	}
	for i := range spec.Walkable {
		spec.Walkable[i] = true
	}
	spec.Obstacles[1*3+1] = worldmodel.ObstacleWaterfall
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("native grid: %v", err)
	}
	return grid
}

func TestNativeWaterfallCrossingOnlyInterceptsUpstreamMovement(t *testing.T) {
	grid := waterfallTestGrid(t, worldmodel.TraversalWater)
	tests := []struct {
		name string
		x, y int
		step world.NativeStep
		want bool
	}{
		{name: "up into waterfall", x: 1, y: 2, step: world.NativeStep{DY: -1}, want: true},
		{name: "down through waterfall", x: 1, y: 0, step: world.NativeStep{DY: 1}},
		{name: "sideways into waterfall", x: 0, y: 1, step: world.NativeStep{DX: 1}},
		{name: "up away from waterfall", x: 2, y: 2, step: world.NativeStep{DY: -1}},
		{name: "ledge hop must not activate field move", x: 1, y: 3, step: world.NativeStep{DY: -2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nativeAscendingWaterfall(grid, tc.x, tc.y, tc.step); got != tc.want {
				t.Fatalf("nativeAscendingWaterfall(%d,%d,%+v) = %v, want %v", tc.x, tc.y, tc.step, got, tc.want)
			}
		})
	}
	if nativeAscendingWaterfall(waterfallTestGrid(t, worldmodel.TraversalLand), 1, 2, world.NativeStep{DY: -1}) {
		t.Fatal("land traversal tried to activate Waterfall")
	}
	if nativeAscendingWaterfall(nil, 1, 2, world.NativeStep{DY: -1}) {
		t.Fatal("nil grid activated Waterfall")
	}
}

func TestWaterfallFieldActionRequiresVerifiedAscent(t *testing.T) {
	spec := FieldMoveSpec{Move: FieldWaterfall, Name: "WATERFALL"}
	start := game.FieldActionState{
		PositionKnown: true,
		NativeMapID:   0x1804,
		X:             6,
		Y:             14,
		Controllable:  true,
	}
	after := start
	after.ActionSucceeded = true

	if fieldActionCompleteFromState(start, after, spec) {
		t.Fatal("queued Waterfall script falsely completed without movement")
	}
	after.Y = 11
	if !fieldActionCompleteFromState(start, after, spec) {
		t.Fatal("completed upward Waterfall movement was rejected")
	}
	for _, tc := range []struct {
		name  string
		alter func(*game.FieldActionState)
	}{
		{"script still in control", func(s *game.FieldActionState) { s.Controllable = false }},
		{"success byte missing", func(s *game.FieldActionState) { s.ActionSucceeded = false }},
		{"different map", func(s *game.FieldActionState) { s.NativeMapID++ }},
		{"different column", func(s *game.FieldActionState) { s.X++ }},
		{"position unavailable", func(s *game.FieldActionState) { s.PositionKnown = false }},
		{"downward displacement", func(s *game.FieldActionState) { s.Y = 15 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := after
			tc.alter(&state)
			if fieldActionCompleteFromState(start, state, spec) {
				t.Fatalf("invalid Waterfall terminal state completed: %+v", state)
			}
		})
	}
	start.PositionKnown = false
	if fieldActionCompleteFromState(start, after, spec) {
		t.Fatal("missing starting position falsely verified Waterfall")
	}
}

func TestWaterfallValidationNeedsSurfing(t *testing.T) {
	spec := FieldMoveSpec{Move: FieldWaterfall, Name: "WATERFALL"}
	if err := validateFieldActionRuntime(game.FieldActionState{}, spec); err == nil {
		t.Fatal("Waterfall from land did not reject invalid context")
	}
	if err := validateFieldActionRuntime(game.FieldActionState{Surfing: true}, spec); err != nil {
		t.Fatalf("Surfing Waterfall was rejected: %v", err)
	}
}
