package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestRoute23VictoryRoadEntranceIsSemanticPivot(t *testing.T) {
	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  route23Map,
		To:    victoryRoad1FMap,
		WarpX: route23VictoryRoadWarpX,
		WarpY: route23VictoryRoadWarpY,
	}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok {
		t.Fatal("Route 23 -> Victory Road 1F entrance is missing semantic transition")
	}
	if transition.ID != "red:route23_league_approach" {
		t.Fatalf("transition id=%q", transition.ID)
	}
	if transition.Gate || transition.PivotOnly {
		t.Fatalf("League approach must be an action pivot, got gate=%v pivot_only=%v", transition.Gate, transition.PivotOnly)
	}
	if len(transition.Requires) != 2 || transition.Requires[0] != capCanSurf || transition.Requires[1] != capCanPassRoute23BadgeChecks {
		t.Fatalf("requirements=%v, want Surf + Route 23 badge checks", transition.Requires)
	}
}

func TestRoute23NorthVictoryRoadExitIsNotCollapsedIntoLeagueApproach(t *testing.T) {
	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  route23Map,
		To:    victoryRoad2FMap,
		WarpX: 14,
		WarpY: 31,
	}
	transition, ok := redAuditedRouteTransitionForEdge(edge)
	if ok && transition.ID == "red:route23_league_approach" {
		t.Fatal("north Route 23 <-> Victory Road 2F exit must remain ordinary geometry")
	}
}
