package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestFieldPathPlanActionCountRejectsOrdinaryWalkBridge(t *testing.T) {
	plan := []fieldPathStep{
		{Move: world.StepRight, Action: fieldPathWalk},
		{Move: world.StepDown, Action: fieldPathWalk},
	}
	if got := fieldPathPlanActionCount(plan); got != 0 {
		t.Fatalf("ordinary walking action count=%d, want 0", got)
	}
}

func TestFieldPathPlanActionCountRecognizesRealBridgeAction(t *testing.T) {
	for _, action := range []fieldPathAction{fieldPathCut, fieldPathSurf, fieldPathForced} {
		plan := []fieldPathStep{
			{Move: world.StepRight, Action: fieldPathWalk},
			{Move: world.StepDown, Action: action},
		}
		if got := fieldPathPlanActionCount(plan); got != 1 {
			t.Fatalf("action %d count=%d, want 1", action, got)
		}
	}
}
