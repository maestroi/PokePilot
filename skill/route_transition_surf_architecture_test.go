package skill

import (
	"os"
	"strings"
	"testing"
)

// Farm #1983 reproduced on Route 19 while taking the southern-sea Surf
// transition toward Cinnabar. The transition must plan the land prefix and
// land->water action together; planning the entire approach as TraversalWater
// makes a valid shore unreachable from land-only components.
func TestSurfTransitionPlansLandToWaterEntryWithFieldPath(t *testing.T) {
	srcBytes, err := os.ReadFile("route_transition.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	start := strings.Index(src, "func (x *redRouteTransitionExecutor) executeSurf")
	if start < 0 {
		t.Fatal("executeSurf function not found")
	}
	end := strings.Index(src[start:], "\nfunc (x *redRouteTransitionExecutor) executeRoute12Snorlax")
	if end < 0 {
		t.Fatal("executeSurf end not found")
	}
	body := src[start : start+end]

	for _, want := range []string{
		"currentFieldPathPlanWithCost(",
		"firstFieldAction(bestPlan)",
		"rejectedSurfEntry",
		"walkWithinMap(x.m, x.romData",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("executeSurf no longer contains %q", want)
		}
	}
	if strings.Contains(body, "world.FindPath(water") {
		t.Fatal("executeSurf regressed to planning the whole shore approach in water mode")
	}
}
