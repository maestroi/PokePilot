package agent

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

func TestPushPuzzleSearchExhaustionNormalizesAsPlanningBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    error
		context string
	}{
		{name: "no solution", kind: world.ErrPushPuzzleNoSolution, context: "no_solution"},
		{name: "state limit", kind: world.ErrPushPuzzleStateLimit, context: "state_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &world.PushPuzzleError{
				Kind:      tc.kind,
				Explored:  42,
				MaxStates: 100,
				Player:    world.Point{X: 6, Y: 15},
			}
			got := normalizeRedFailure(
				gameruntime.FailurePhaseExecution,
				err,
				Observation{Controllable: true},
			)
			if got.Class != gameruntime.FailureClassBlocked || got.Cause != "push_puzzle_search_exhausted" || !got.Recoverable {
				t.Fatalf("failure = %+v; want recoverable blocked push-puzzle search exhaustion", got)
			}
			if len(got.Context) != 1 || got.Context[0] != tc.context {
				t.Fatalf("context = %v; want [%s]", got.Context, tc.context)
			}
		})
	}
}

func TestPushPuzzleSearchExhaustionUnsafeBoundaryStillFailsClosed(t *testing.T) {
	err := &world.PushPuzzleError{
		Kind:      world.ErrPushPuzzleNoSolution,
		Explored:  42,
		MaxStates: 100,
	}
	got := normalizeRedFailure(gameruntime.FailurePhaseExecution, err, Observation{})
	if got.Class != gameruntime.FailureClassControllerUncertain || got.Recoverable {
		t.Fatalf("failure = %+v; want non-recoverable controller uncertainty", got)
	}
	if got.Cause != "push_puzzle_search_exhausted" {
		t.Fatalf("cause = %q; want push_puzzle_search_exhausted", got.Cause)
	}
}

func TestRunFailurePolicyPushPuzzleSearchDoesNotSpendMechanicalBudget(t *testing.T) {
	policy := newRunFailurePolicy(2)
	obj := Objective{Kind: KindProgress, Progress: "victory_road_cleared"}
	result := ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "push_puzzle_search_exhausted",
			Context:     []string{"no_solution"},
			Recoverable: true,
		},
		Final: Observation{Location: "victory road 3f", X: 6, Y: 15, Controllable: true},
	}

	for i := 0; i < 5; i++ {
		got := policy.recoverable(obj, result, true, 0)
		if got.Stop != StopUnset || !got.Recovered || got.ReplanReason != "objective_failed" {
			t.Fatalf("push-puzzle exhaustion %d = %+v; want planning replan without fatal-budget spend", i+1, got)
		}
	}

	mechanical := result
	mechanical.Failure = &gameruntime.Failure{
		Class:       gameruntime.FailureClassBlocked,
		Cause:       "menu_stuck",
		Recoverable: true,
	}
	if got := policy.recoverable(obj, mechanical, true, 0); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("first mechanical failure after puzzle search = %+v; want recovered", got)
	}
	if got := policy.recoverable(obj, mechanical, true, 0); got.Stop != StopFailed {
		t.Fatalf("repeated mechanical failure = %+v; want StopFailed", got)
	}
}

func TestRunFailurePolicyBlockedPurchaseDoesNotSpendMechanicalBudget(t *testing.T) {
	for _, cause := range []string{"outcome:blocked", "cant_afford", "not_in_stock", "bag_not_risen"} {
		t.Run(cause, func(t *testing.T) {
			policy := newRunFailurePolicy(2)
			obj := Objective{Kind: KindBuy, Item: ItemID("pokeball"), Qty: 5}
			result := ObjectiveResult{
				Objective: obj,
				Outcome:   OutcomeBlocked,
				Failure: &gameruntime.Failure{
					Class:       gameruntime.FailureClassBlocked,
					Cause:       cause,
					Recoverable: true,
				},
				Final: Observation{Location: "viridian mart", Money: 100, Controllable: true},
			}

			for i := 0; i < 5; i++ {
				got := policy.recoverable(obj, result, true, 0)
				if got.Stop != StopUnset || !got.Recovered || got.ReplanReason != "objective_failed" {
					t.Fatalf("purchase blockage %d = %+v; want planning replan without fatal-budget spend", i+1, got)
				}
			}

			mechanical := result
			mechanical.Failure = &gameruntime.Failure{
				Class:       gameruntime.FailureClassBlocked,
				Cause:       "shop_controller_stalled",
				Recoverable: true,
			}
			if got := policy.recoverable(obj, mechanical, true, 0); got.Stop != StopUnset || !got.Recovered {
				t.Fatalf("first shop controller failure after resource blockages = %+v; want recovered", got)
			}
			if got := policy.recoverable(obj, mechanical, true, 0); got.Stop != StopFailed {
				t.Fatalf("repeated shop controller failure = %+v; want StopFailed", got)
			}
		})
	}
}

// Farm triage e0135a8729e5e671 / #2305: Vermilion Gym nested EnsureProgressionPokeBalls
// during Cut-carrier repair and surfaced cant_afford while still labeled as the
// gym objective. KindBuy already spared the mechanical budget; KindGym must too,
// or two matching money shortages exhaust recovery and open the circuit.
func TestRunFailurePolicyGymCantAffordDoesNotSpendMechanicalBudget(t *testing.T) {
	policy := newRunFailurePolicy(2)
	obj := Objective{Kind: KindGym, Place: "vermilion gym"}
	result := ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "cant_afford",
			Recoverable: true,
			Context:     []string{"vermilion gym"},
		},
		Final: Observation{Location: "vermilion mart", Map: 0x5b, Money: 50, Controllable: true},
	}

	for i := 0; i < 5; i++ {
		got := policy.recoverable(obj, result, true, 0)
		if got.Stop != StopUnset || !got.Recovered || got.ReplanReason != "objective_failed" {
			t.Fatalf("gym cant_afford %d = %+v; want planning replan without fatal-budget spend", i+1, got)
		}
	}

	mechanical := result
	mechanical.Failure = &gameruntime.Failure{
		Class:       gameruntime.FailureClassBlocked,
		Cause:       "menu_stuck",
		Recoverable: true,
	}
	if got := policy.recoverable(obj, mechanical, true, 0); got.Stop != StopUnset || !got.Recovered {
		t.Fatalf("first mechanical failure after gym cant_afford = %+v; want recovered", got)
	}
	if got := policy.recoverable(obj, mechanical, true, 0); got.Stop != StopFailed {
		t.Fatalf("repeated mechanical failure = %+v; want StopFailed", got)
	}
}
