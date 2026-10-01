package agent

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func TestDecisionObjectivePlannerCanOnlyReturnOfferedObjective(t *testing.T) {
	offered := []Objective{
		{Kind: KindGoTo, Place: "pallet town"},
		{Kind: KindHeal},
	}
	planner := &DecisionObjectivePlanner{
		Engine: fixedDecisionEngine{resp: DecisionResponse{
			Choice:        "2",
			Probabilities: map[string]float64{"1": 0.1, "2": 0.9},
		}},
	}
	got, err := planner.Next(Observation{}, offered)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.String() != offered[1].String() {
		t.Fatalf("choice = %s, want %s", got, offered[1])
	}
}

func TestDecisionObjectivePlannerRejectsLowConfidence(t *testing.T) {
	offered := []Objective{
		{Kind: KindGoTo, Place: "pallet town"},
		{Kind: KindHeal},
	}
	planner := &DecisionObjectivePlanner{
		Engine: fixedDecisionEngine{resp: DecisionResponse{
			Choice:        "2",
			Probabilities: map[string]float64{"1": 0.4, "2": 0.6},
		}},
		MinConfidence: 0.7,
	}
	if _, err := planner.Next(Observation{}, offered); !errors.Is(err, ErrDecisionLowConfidence) {
		t.Fatalf("error = %v, want ErrDecisionLowConfidence", err)
	}
}

func TestFailureDecisionStopAllowedKeepsCombatDefeatRecoverable(t *testing.T) {
	combat := ObjectiveResult{
		Objective: Objective{Kind: KindGym, Place: PlaceID("cerulean gym")},
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       failureCauseCombatDefeat,
			Recoverable: true,
		},
	}
	if FailureDecisionStopAllowed(combat) {
		t.Fatal("combat defeat allowed typed decision to bypass deterministic recovery")
	}
	if stop, err := failureDecisionStopsRun(combat, "pause"); err != nil || stop {
		t.Fatalf("combat pause => stop=%v err=%v, want deterministic recovery", stop, err)
	}

	navigation := ObjectiveResult{
		Objective: Objective{Kind: KindGoTo, Place: PlaceID("pewter city")},
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "no_path",
			Recoverable: true,
		},
	}
	if !FailureDecisionStopAllowed(navigation) {
		t.Fatal("non-combat recoverable failure unexpectedly forbids conservative typed stop")
	}
	if stop, err := failureDecisionStopsRun(navigation, "pause"); err != nil || !stop {
		t.Fatalf("navigation pause => stop=%v err=%v, want conservative stop allowed", stop, err)
	}
}

func TestFailureDecisionRequestIsBoundedAndConservative(t *testing.T) {
	req, err := FailureDecisionRequest(ObjectiveResult{
		Objective: Objective{Kind: KindGoTo, Place: "pewter city"},
		Outcome:   OutcomeBlocked,
	})
	if err != nil {
		t.Fatalf("FailureDecisionRequest: %v", err)
	}
	seen := map[string]bool{}
	for _, choice := range req.Choices {
		seen[choice.ID] = true
	}
	for _, want := range []string{"retry", "recover", "replan", "pause", "impossible", "unknown"} {
		if !seen[want] {
			t.Fatalf("missing failure choice %q from %#v", want, req.Choices)
		}
	}
	for _, choice := range []string{"retry", "recover", "replan", "unknown"} {
		stop, err := FailureDecisionStops(choice)
		if err != nil || stop {
			t.Fatalf("%q => stop=%v err=%v, want continue", choice, stop, err)
		}
	}
	for _, choice := range []string{"pause", "impossible"} {
		stop, err := FailureDecisionStops(choice)
		if err != nil || !stop {
			t.Fatalf("%q => stop=%v err=%v, want stop", choice, stop, err)
		}
	}
}
