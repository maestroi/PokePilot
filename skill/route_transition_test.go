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
		{world.Edge{From: victoryRoad1FMap, To: victoryRoad2FMap}, VictoryRoad1FSwitch, true},
		{world.Edge{From: victoryRoad2FMap, To: victoryRoad3FMap}, VictoryRoad2FSwitch1, true},
		{world.Edge{From: victoryRoad3FMap, To: victoryRoad2FMap}, VictoryRoad3FSwitch, true},
		{world.Edge{From: victoryRoad2FMap, To: victoryRoad1FMap}, 0, false},
	}
	for _, tt := range tests {
		got, ok := victoryRoadSectionForTransition(tt.edge)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("edge %02x->%02x = (%v,%v), want (%v,%v)", tt.edge.From, tt.edge.To, got, ok, tt.want, tt.ok)
		}
	}
}

func TestDirectStrengthTransitionRequiresBattlePolicyBeforeSolver(t *testing.T) {
	if !errors.Is(ErrRouteTransitionNeedsBattlePolicy, ErrRouteTransitionNeedsBattlePolicy) {
		t.Fatal("policy sentinel lost identity")
	}
}
