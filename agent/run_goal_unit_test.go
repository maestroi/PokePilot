package agent

import (
	"errors"
	"testing"
)

func TestIncompleteGoalErrorIsTypedWithoutROM(t *testing.T) {
	status := GoalStatus{Summary: "badges 0/1", Current: 0, Target: 1}
	err := incompleteGoalError(status)
	if !errors.Is(err, ErrGoalIncomplete) {
		t.Fatalf("err = %v, want ErrGoalIncomplete", err)
	}
}

func TestResolveRunGoalKeepsFreeTextPromptOnly(t *testing.T) {
	goal, deterministic, err := resolveRunGoal(NewScriptedPlanner(), "Explore Kanto and see how far you get.")
	if err != nil {
		t.Fatalf("resolveRunGoal: %v", err)
	}
	if deterministic || goal.Kind != GoalNone {
		t.Fatalf("free-text goal = (%+v, deterministic=%v), want prompt-only", goal, deterministic)
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
