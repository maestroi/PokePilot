package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

// A long cave/grind leg can expose several independent runtime blockers in
// sequence. Each one needs two confirmations before it becomes a call-local
// blocker. The old total-attempt budget died on the fourth such tile even
// though every pair of retries taught the pathfinder something new.
func TestWalkAroundRetryBudgetResetsWhenLearningNewBlockers(t *testing.T) {
	defects := []*ErrBlocked{
		blockedAt(10, 22, world.StepLeft), // destination (9,22)
		blockedAt(9, 22, world.StepUp),    // destination (9,21)
		blockedAt(9, 21, world.StepLeft),  // destination (8,21)
		blockedAt(8, 21, world.StepDown),  // destination (8,22)
	}

	chosen := -1
	plans := 0
	walks := 0
	err := walkAround(
		func() map[[2]int]bool { return map[[2]int]bool{} },
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			plans++
			chosen = -1
			for i, defect := range defects {
				if !blocked[blockedDestination(defect)] {
					chosen = i
					return []world.Step{defect.Step}, nil
				}
			}
			return []world.Step{world.StepRight}, nil
		},
		func([]world.Step) error {
			walks++
			if chosen >= 0 {
				return defects[chosen]
			}
			return nil
		},
		func() {},
	)
	if err != nil {
		t.Fatalf("walkAround: %v", err)
	}
	if plans != len(defects)*unexplainedBlockLearnThreshold+1 {
		t.Fatalf("plans = %d, want %d (two confirmations per blocker plus final reroute)", plans, len(defects)*unexplainedBlockLearnThreshold+1)
	}
	if walks != plans {
		t.Fatalf("walks = %d, plans = %d", walks, plans)
	}
	if plans <= maxWalkRetries+1 {
		t.Fatalf("test did not cross the old total retry budget: plans=%d max=%d", plans, maxWalkRetries+1)
	}
}
