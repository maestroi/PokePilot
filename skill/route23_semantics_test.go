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

func TestRoute23SouthReturnIsSemanticPivot(t *testing.T) {
	for _, x := range []uint8{7, 8} {
		edge := world.Edge{
			Kind:  world.EdgeWarp,
			From:  route23Map,
			To:    route22GateMap,
			WarpX: x,
			WarpY: route23Route22GateWarpY,
		}
		transition, ok := redRouteTransitionForEdge(edge)
		if !ok {
			t.Fatalf("Route 23 south warp x=%d is missing semantic return transition", x)
		}
		if transition.ID != "red:route23_league_return" {
			t.Fatalf("x=%d transition id=%q", x, transition.ID)
		}
		if transition.Gate || transition.PivotOnly || transition.PortBypass {
			t.Fatalf("League return must be a source action pivot, got gate=%v pivot_only=%v port_bypass=%v",
				transition.Gate, transition.PivotOnly, transition.PortBypass)
		}
		if len(transition.Requires) != 2 || transition.Requires[0] != capCanSurf || transition.Requires[1] != capCanPassRoute23BadgeChecks {
			t.Fatalf("x=%d requirements=%v, want Surf + Route 23 badge checks", x, transition.Requires)
		}
	}

	// Entering Route 23 from the gate remains ordinary geometry. The semantic
	// action exists only to bridge the north-side water-separated component
	// toward the south exit.
	reverse := world.Edge{
		Kind:  world.EdgeWarp,
		From:  route22GateMap,
		To:    route23Map,
		WarpX: 4,
		WarpY: 0,
	}
	if transition, ok := redRouteTransitionForEdge(reverse); ok && transition.ID == "red:route23_league_return" {
		t.Fatalf("Route 22 Gate -> Route 23 unexpectedly owns League return action: %+v", transition)
	}
}

func TestRoute23LeagueReturnUsesCaveOnlyFromNorthComponent(t *testing.T) {
	for _, tc := range []struct {
		x, y uint8
		want bool
	}{
		{14, 31, true},  // 2F exit door
		{18, 30, true},  // north exterior
		{14, 37, true},  // east exit pocket
		{4, 31, false},  // 1F entrance door
		{4, 32, false},  // south cave component
		{7, 138, false}, // Route 22 gate approach
	} {
		if got := route23LeagueReturnNeedsVictoryRoad(tc.x, tc.y); got != tc.want {
			t.Errorf("Route 23 (%d,%d) needs reverse cave=%v, want %v", tc.x, tc.y, got, tc.want)
		}
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
