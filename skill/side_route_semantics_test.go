package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestRoute2GateWarpUsesCutOnlyAsComponentPivot(t *testing.T) {
	for _, point := range [][2]uint8{{16, 35}, {15, 39}} {
		edge := world.Edge{Kind: world.EdgeWarp, From: semanticRoute2Map, To: route2GateMap, WarpX: point[0], WarpY: point[1]}
		transition, ok := redRouteTransitionForEdge(edge)
		if !ok {
			t.Fatalf("Route 2 Gate warp (%d,%d) is missing semantic transition", point[0], point[1])
		}
		if transition.ID != "red:route2_gate_cut" || !transition.PivotOnly || !transition.PortBypass || transition.Gate {
			t.Fatalf("Route 2 Gate transition=%+v", transition)
		}
		if len(transition.Requires) != 1 || transition.Requires[0] != capCanCut {
			t.Fatalf("Route 2 Gate requirements=%v, want Cut", transition.Requires)
		}
	}
}

func TestPowerPlantWarpUsesSurfOnlyAsComponentPivot(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeWarp, From: route10Map, To: powerPlantMap, WarpX: powerPlantWarpX, WarpY: powerPlantWarpY}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok {
		t.Fatal("Route 10 -> Power Plant warp is missing semantic transition")
	}
	if transition.ID != "red:power_plant_surf" || !transition.PivotOnly || !transition.PortBypass || transition.Gate {
		t.Fatalf("Power Plant transition=%+v", transition)
	}
	if len(transition.Requires) != 1 || transition.Requires[0] != capCanSurf {
		t.Fatalf("Power Plant requirements=%v, want Surf", transition.Requires)
	}
}

func TestCeruleanCaveB1FWarpUsesSurfOnlyAsComponentPivot(t *testing.T) {
	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  ceruleanCave1FMap,
		To:    ceruleanCaveB1FMap,
		WarpX: ceruleanCaveB1FWarpX,
		WarpY: ceruleanCaveB1FWarpY,
	}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok {
		t.Fatal("Cerulean Cave 1F -> B1F warp is missing semantic transition")
	}
	if transition.ID != "red:cerulean_cave_b1f_surf" || !transition.PivotOnly || !transition.PortBypass || transition.Gate {
		t.Fatalf("Cerulean Cave transition=%+v", transition)
	}
	if len(transition.Requires) != 1 || transition.Requires[0] != capCanSurf {
		t.Fatalf("Cerulean Cave requirements=%v, want Surf", transition.Requires)
	}
}
