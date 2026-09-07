package agent

import (
	"errors"
	"testing"
)

func TestClassifyPlannerDoneRejectsIncompleteDeterministicGoal(t *testing.T) {
	status := GoalStatus{Summary: "badges 0/1", Current: 0, Target: 1}
	stop, err := classifyPlannerDone(true, status)

	if stop != StopError {
		t.Fatalf("stop = %d, want StopError", stop)
	}
	if !errors.Is(err, ErrGoalIncomplete) {
		t.Fatalf("err = %v, want ErrGoalIncomplete", err)
	}
}

func TestClassifyPlannerDoneKeepsPromptOnlySemantics(t *testing.T) {
	stop, err := classifyPlannerDone(false, GoalStatus{})
	if stop != StopDone || err != nil {
		t.Fatalf("prompt-only planner done = (%d, %v), want (StopDone, nil)", stop, err)
	}
}

func TestResolveRunGoalRequiresChampionStateForLeaguePreset(t *testing.T) {
	p := &LLMPlanner{Goal: "Beat the Elite Four and Champion."}
	goal, deterministic, err := resolveRunGoal(p, "")
	if err != nil {
		t.Fatalf("resolveRunGoal: %v", err)
	}
	if !deterministic {
		t.Fatal("League preset resolved as prompt-only")
	}
	if status := EvaluateGoal(goal, Observation{}); status.Complete {
		t.Fatalf("League goal completed without Champion event: %+v", status)
	}
}
