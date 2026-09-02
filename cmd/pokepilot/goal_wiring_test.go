package main

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestStatsPlannerStopsOnCompletedStructuredGoal(t *testing.T) {
	inner := &agent.LLMPlanner{Goal: "badges:1"}
	p := newStatsPlanner(inner, nil, nil)

	_, err := p.Next(agent.Observation{Badges: []string{"Boulder"}}, nil)
	if !errors.Is(err, agent.ErrDone) {
		t.Fatalf("Next error = %v, want ErrDone", err)
	}
	if p.stats.Calls != 0 {
		t.Fatalf("model calls = %d, want 0 after deterministic completion", p.stats.Calls)
	}
}

func TestStatsPlannerLeavesFreeTextGoalPromptOnly(t *testing.T) {
	inner := &agent.LLMPlanner{Goal: "Earn the Boulder Badge."}
	p := newStatsPlanner(inner, nil, nil)

	done, err := p.goalDone(agent.Observation{Badges: []string{"Boulder"}})
	if err != nil {
		t.Fatalf("goalDone: %v", err)
	}
	if done {
		t.Fatal("free-text planner goal unexpectedly became a deterministic stop")
	}
}

func TestStatsPlannerRejectsMalformedStructuredGoal(t *testing.T) {
	inner := &agent.LLMPlanner{Goal: "badges:99"}
	p := newStatsPlanner(inner, nil, nil)

	_, err := p.Next(agent.Observation{}, nil)
	if err == nil || errors.Is(err, agent.ErrDone) {
		t.Fatalf("Next error = %v, want structured-goal validation error", err)
	}
}
