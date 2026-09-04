package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestStatsPlannerStopsOnCompletedStructuredGoal(t *testing.T) {
	var (
		pushed int
		got    runStats
	)
	p := newStatsPlanner("", "badges:1", nil, func(v any) {
		pushed++
		got = v.(runStats)
	}, nil)

	_, err := p.Next(agent.Observation{Round: 4, RoundsLeft: 20, Badges: []string{"Boulder"}}, nil)
	if !errors.Is(err, agent.ErrDone) {
		t.Fatalf("Next error = %v, want ErrDone", err)
	}
	if p.stats.Calls != 0 {
		t.Fatalf("model calls = %d, want 0 after deterministic completion", p.stats.Calls)
	}
	if pushed != 1 {
		t.Fatalf("final goal snapshot pushes = %d, want 1", pushed)
	}
	if !got.GoalComplete || got.GoalSummary != "badges 1/1" || got.GoalCurrent != 1 || got.GoalTarget != 1 {
		t.Fatalf("final goal stats = %+v", got)
	}
}

func TestStatsPlannerStopsOnCompletedGoalPreset(t *testing.T) {
	p := newStatsPlanner("", "Earn the Boulder Badge.", nil, nil, nil)

	_, err := p.Next(agent.Observation{Round: 4, RoundsLeft: 20, Badges: []string{"Boulder"}}, nil)
	if !errors.Is(err, agent.ErrDone) {
		t.Fatalf("Next error = %v, want ErrDone", err)
	}
	if p.stats.Calls != 0 {
		t.Fatalf("model calls = %d, want 0 after preset completion", p.stats.Calls)
	}
	if !p.stats.GoalComplete || p.stats.GoalSummary != "badges 1/1" || p.stats.GoalCurrent != 1 || p.stats.GoalTarget != 1 {
		t.Fatalf("preset goal stats = %+v", p.stats)
	}
}

func TestStatsPlannerSurfacesStructuredGoalProgress(t *testing.T) {
	p := newStatsPlanner("", "badges:2", nil, nil, nil)
	p.inner.ExtraSystem = "baseline system note"
	p.baseExtraSystem = p.inner.ExtraSystem

	done, err := p.prepareRunContext(agent.Observation{
		Round: 1, Badges: []string{"Boulder"}, Party: []agent.PartyMon{{Level: 12}},
	})
	if err != nil {
		t.Fatalf("prepareRunContext: %v", err)
	}
	if done {
		t.Fatal("badges:2 unexpectedly complete at one badge")
	}
	if p.stats.GoalSummary != "badges 1/2" || p.stats.GoalCurrent != 1 || p.stats.GoalTarget != 2 || p.stats.GoalComplete {
		t.Fatalf("goal stats = %+v", p.stats)
	}
	if !strings.Contains(p.inner.ExtraSystem, "RUN GOAL STATUS: badges 1/2") {
		t.Fatalf("structured goal status not added to planner context: %q", p.inner.ExtraSystem)
	}
	if !strings.HasPrefix(p.inner.ExtraSystem, "baseline system note\n\n") {
		t.Fatalf("base ExtraSystem not preserved: %q", p.inner.ExtraSystem)
	}
	if strings.Contains(p.inner.ExtraSystem, "go to") || strings.Contains(p.inner.ExtraSystem, "train") {
		t.Fatalf("goal progress note prescribed a strategy: %q", p.inner.ExtraSystem)
	}
}

func TestStatsPlannerLeavesArbitraryFreeTextGoalPromptOnly(t *testing.T) {
	p := newStatsPlanner("", "Explore Kanto and see how far you get.", nil, nil, nil)
	p.inner.ExtraSystem = "baseline"
	p.baseExtraSystem = p.inner.ExtraSystem

	if done, err := p.prepareRunContext(agent.Observation{Round: 1, Badges: []string{"Boulder"}}); err != nil || done {
		t.Fatalf("prepareRunContext = done %v, err %v; want arbitrary free text prompt-only", done, err)
	}
	if p.stats.GoalSummary != "" || p.stats.GoalCurrent != 0 || p.stats.GoalTarget != 0 || p.stats.GoalComplete {
		t.Fatalf("free-text goal leaked into deterministic stats: %+v", p.stats)
	}
	if p.inner.ExtraSystem != "baseline" {
		t.Fatalf("free-text goal changed system context: %q", p.inner.ExtraSystem)
	}
}

func TestStatsPlannerRejectsMalformedStructuredGoal(t *testing.T) {
	p := newStatsPlanner("", "badges:99", nil, nil, nil)

	_, err := p.Next(agent.Observation{}, nil)
	if err == nil || errors.Is(err, agent.ErrDone) {
		t.Fatalf("Next error = %v, want structured-goal validation error", err)
	}
}
