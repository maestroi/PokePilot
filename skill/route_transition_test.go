package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestVictoryRoadTransitionDelegatesOnlyOwnedStrengthSections(t *testing.T) {
	tests := []struct {
		edge world.Edge
		want VictoryRoadBoulderSection
		ok   bool
	}{
		{world.Edge{From: victoryRoad1FMap, To: victoryRoad2FMap, WarpX: 1, WarpY: 1}, VictoryRoad1FSwitch, true},
		{world.Edge{From: victoryRoad2FMap, To: victoryRoad3FMap, WarpX: 23, WarpY: 7}, VictoryRoad2FSwitch1, true},
		{world.Edge{From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 2, WarpY: 0}, VictoryRoad3FSwitch, true},

		// Sibling ladders connect the same floors but do not own these switches.
		// Annotating them grants skipCanExit and can invoke an unreachable puzzle
		// solver instead of simply traversing the live ladder (#2233).
		{world.Edge{From: victoryRoad2FMap, To: victoryRoad3FMap, WarpX: 25, WarpY: 14}, 0, false},
		{world.Edge{From: victoryRoad2FMap, To: victoryRoad3FMap, WarpX: 27, WarpY: 7}, 0, false},
		{world.Edge{From: victoryRoad2FMap, To: victoryRoad3FMap, WarpX: 1, WarpY: 1}, 0, false},
		{world.Edge{From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 23, WarpY: 7}, 0, false},
		{world.Edge{From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 26, WarpY: 8}, 0, false},
		{world.Edge{From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 27, WarpY: 15}, 0, false},
		{world.Edge{From: victoryRoad2FMap, To: victoryRoad1FMap, WarpX: 0, WarpY: 8}, 0, false},
	}
	for _, tt := range tests {
		got, ok := victoryRoadSectionForTransition(tt.edge)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("edge %02x->%02x warp(%d,%d) = (%v,%v), want (%v,%v)",
				t.edge.From, tt.edge.To, tt.edge.WarpX, tt.edge.WarpY, got, ok, tt.want, tt.ok)
		}
	}
}

func TestDirectStrengthTransitionRequiresBattlePolicyBeforeSolver(t *testing.T) {
	if !errors.Is(ErrRouteTransitionNeedsBattlePolicy, ErrRouteTransitionNeedsBattlePolicy) {
		t.Fatal("policy sentinel lost identity")
	}
}
