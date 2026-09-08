package main

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestStatsPlannerMirrorsCompletedRuntimeGoalOnce(t *testing.T) {
	var (
		pushed int
		got    runStats
	)
	p := newStatsPlanner("", "", "badges:1", nil, func(v any) {
		pushed++
		got = v.(runStats)
	}, nil)
	obs := agent.Observation{Round: 4, RoundsLeft: 20, Badges: []string{"Boulder"}}
	status := agent.GoalStatus{Complete: true, Summary: "badges 1/1", Current: 1, Target: 1}

	p.ObserveRunGoal(obs, status, true)
	// Run may sample the same settled final state again when constructing its
	// final diagnostics. Mirroring that must not fabricate another UI event.
	p.ObserveRunGoal(obs, status, true)

	if p.stats.Calls != 0 {
		t.Fatalf("model calls = %d, want 0 after runtime-owned completion", p.stats.Calls)
	}
	if pushed != 1 {
		t.Fatalf("final goal snapshot pushes = %d, want 1", pushed)
	}
	if !got.GoalComplete || got.GoalSummary != "badges 1/1" || got.GoalCurrent != 1 || got.GoalTarget != 1 {
		t.Fatalf("final goal stats = %+v", got)
	}
}

func TestStatsPlannerExposesRawRunGoal(t *testing.T) {
	p := newStatsPlanner("", "", "Earn the Boulder Badge.", nil, nil, nil)
	if got := p.RunGoal(); got != "Earn the Boulder Badge." {
		t.Fatalf("RunGoal = %q", got)
	}
}

func TestStatsPlannerSurfacesRuntimeGoalProgress(t *testing.T) {
	p := newStatsPlanner("", "", "badges:2", nil, nil, nil)
	p.inner.ExtraSystem = "baseline system note"
	p.baseExtraSystem = p.inner.ExtraSystem
	obs := agent.Observation{
		Round: 1, Badges: []string{"Boulder"}, Party: []agent.PartyMon{{Level: 12}},
	}

	p.ObserveRunGoal(obs, agent.GoalStatus{Summary: "badges 1/2", Current: 1, Target: 2}, true)
	p.prepareRunContext(obs)

	if p.stats.GoalSummary != "badges 1/2" || p.stats.GoalCurrent != 1 || p.stats.GoalTarget != 2 || p.stats.GoalComplete {
		t.Fatalf("goal stats = %+v", p.stats)
	}
	if !strings.Contains(p.inner.ExtraSystem, "RUN GOAL STATUS: badges 1/2") {
		t.Fatalf("runtime goal status not added to planner context: %q", p.inner.ExtraSystem)
	}
	if !strings.HasPrefix(p.inner.ExtraSystem, "baseline system note\n\n") {
		t.Fatalf("base ExtraSystem not preserved: %q", p.inner.ExtraSystem)
	}
	if strings.Contains(p.inner.ExtraSystem, "go to") || strings.Contains(p.inner.ExtraSystem, "train") {
		t.Fatalf("goal progress note prescribed a strategy: %q", p.inner.ExtraSystem)
	}
}

func TestStatsPlannerLeavesPromptOnlyGoalOutOfDeterministicStats(t *testing.T) {
	p := newStatsPlanner("", "", "Explore Kanto and see how far you get.", nil, nil, nil)
	p.inner.ExtraSystem = "baseline"
	p.baseExtraSystem = p.inner.ExtraSystem
	obs := agent.Observation{Round: 1, Badges: []string{"Boulder"}}

	p.ObserveRunGoal(obs, agent.GoalStatus{}, false)
	p.prepareRunContext(obs)

	if p.stats.GoalSummary != "" || p.stats.GoalCurrent != 0 || p.stats.GoalTarget != 0 || p.stats.GoalComplete {
		t.Fatalf("prompt-only goal leaked into deterministic stats: %+v", p.stats)
	}
	if p.inner.ExtraSystem != "baseline" {
		t.Fatalf("prompt-only goal changed system context: %q", p.inner.ExtraSystem)
	}
}
