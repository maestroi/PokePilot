package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestForcedMovementTablesMatchRedScripts(t *testing.T) {
	tests := []struct {
		name      string
		mapID     uint8
		wantCount int
		checks    map[[2]int]world.Point
	}{
		{
			name:      "Rocket Hideout B2F",
			mapID:     forcedRocketHideoutB2F,
			wantCount: 43,
			checks: map[[2]int]world.Point{
				{4, 9}:   {X: 2, Y: 9},
				{11, 14}: {X: 15, Y: 18},
				{17, 11}: {X: 2, Y: 9},
			},
		},
		{
			name:      "Rocket Hideout B3F",
			mapID:     forcedRocketHideoutB3F,
			wantCount: 16,
			checks: map[[2]int]world.Point{
				{10, 13}: {X: 14, Y: 13},
				{15, 18}: {X: 15, Y: 22},
			},
		},
		{
			name:      "Viridian Gym",
			mapID:     forcedViridianGym,
			wantCount: 12,
			checks: map[[2]int]world.Point{
				{19, 11}: {X: 19, Y: 2},
				{4, 6}:   {X: 4, Y: 13},
				{13, 17}: {X: 1, Y: 17},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ForcedMovementTransitions(tc.mapID)
			if len(got) != tc.wantCount {
				t.Fatalf("transition count = %d, want %d", len(got), tc.wantCount)
			}
			for trigger, want := range tc.checks {
				landing, ok := ForcedMovementLanding(tc.mapID, trigger[0], trigger[1])
				if !ok || landing != want {
					t.Fatalf("trigger %v = %+v,%v; want %+v", trigger, landing, ok, want)
				}
			}
		})
	}
}

func TestForcedMovementUnknownMapIsOrdinary(t *testing.T) {
	if _, ok := ForcedMovementLanding(0xC7, 4, 9); ok {
		t.Fatal("Rocket Hideout B1F unexpectedly has forced-movement transitions")
	}
	if got := ForcedMovementTransitions(0xC7); len(got) != 0 {
		t.Fatalf("unknown map transitions = %v, want none", got)
	}
}
