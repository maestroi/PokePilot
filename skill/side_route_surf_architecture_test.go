package skill

import (
	"os"
	"strings"
	"testing"
)

// Farm #2288 failed the Route 10 -> Power Plant semantic transition before
// Traverse could enter the warp. Surf-to-warp pivots must stay capability-only:
// Traverse's shared warp field-path approach owns the land prefix, Surf action,
// live-grid refresh, and concrete warp retry.
func TestSideRouteSurfWarpDelegatesFieldExecutionToTraverse(t *testing.T) {
	sideBytes, err := os.ReadFile("side_route_semantics.go")
	if err != nil {
		t.Fatal(err)
	}
	side := string(sideBytes)
	start := strings.Index(side, "func (x *redRouteTransitionExecutor) executeSideRouteTransition")
	if start < 0 {
		t.Fatal("executeSideRouteTransition function not found")
	}
	end := strings.Index(side[start:], "\nfunc (x *redRouteTransitionExecutor) liveTransitionBlockage")
	if end < 0 {
		t.Fatal("executeSideRouteTransition end not found")
	}
	body := side[start : start+end]

	for _, id := range []string{"red:power_plant_surf", "red:cerulean_cave_b1f_surf"} {
		if !strings.Contains(body, id) {
			t.Fatalf("side-route executor no longer owns %q capability check", id)
		}
	}
	if strings.Contains(side, "executeSurfWarpApproach") || strings.Contains(side, "TraversalWater") {
		t.Fatal("Surf warp pivot regressed to a transition-local water-mode approach")
	}

	warpBytes, err := os.ReadFile("warp.go")
	if err != nil {
		t.Fatal(err)
	}
	warp := string(warpBytes)
	if !strings.Contains(warp, "aperr := approachWarpWithFieldPath(m, romData, e, extraBlocked)") {
		t.Fatal("Traverse no longer delegates unreachable warp approaches to the shared field-path planner")
	}
	if !strings.Contains(warp, "grid = g") || !strings.Contains(warp, "candidate--") {
		t.Fatal("Traverse no longer refreshes live topology and retries the warp after a field-path approach")
	}
}
