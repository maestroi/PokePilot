package main

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestStatsPlannerSurfacesReplanSignalFromCarriedIntent(t *testing.T) {
	inner := &agent.LLMPlanner{ExtraSystem: "baseline system note"}
	p := newStatsPlanner(inner, nil, nil, nil)

	for round := 1; round <= strategicReplanAfter+1; round++ {
		p.prepareRunContext(agent.Observation{
			Round:  round,
			Map:    1,
			Intent: "find a way through viridian forest",
			Party:  []agent.PartyMon{{Level: 8}},
		})
	}

	if !strings.Contains(inner.ExtraSystem, `Intent "find a way through viridian forest"`) {
		t.Fatalf("replan signal did not reuse carried intent: %q", inner.ExtraSystem)
	}
	if !strings.Contains(inner.ExtraSystem, "materially different approach") {
		t.Fatalf("replan signal = %q", inner.ExtraSystem)
	}
	if !strings.HasPrefix(inner.ExtraSystem, "baseline system note\n\n") {
		t.Fatalf("existing ExtraSystem was not preserved: %q", inner.ExtraSystem)
	}
}

func TestStatsPlannerSurfacesReplanWhenOnlyLevelsAdvance(t *testing.T) {
	inner := &agent.LLMPlanner{}
	p := newStatsPlanner(inner, nil, nil, nil)

	for round := 1; round <= strategicReplanAfter+1; round++ {
		p.prepareRunContext(agent.Observation{
			Round:  round,
			Map:    2,
			Intent: "keep training near pewter",
			Badges: []string{"boulder"},
			Party:  []agent.PartyMon{{Level: uint8(15 + round)}},
		})
	}

	if p.strategy.NoProgress != 0 {
		t.Fatalf("level gains should remain measurable progress: %d", p.strategy.NoProgress)
	}
	if p.strategy.NoWorldProgress < strategicReplanAfter {
		t.Fatalf("world-progress counter = %d, want >= %d", p.strategy.NoWorldProgress, strategicReplanAfter)
	}
	if !strings.Contains(inner.ExtraSystem, "RUN REPLAN SIGNAL") || !strings.Contains(inner.ExtraSystem, "no new badge, event, or map progress") {
		t.Fatalf("level-only world stall did not reach planner: %q", inner.ExtraSystem)
	}
}

func TestStatsPlannerCountsOneProgressSamplePerRound(t *testing.T) {
	p := newStatsPlanner(&agent.LLMPlanner{}, nil, nil, nil)
	obs := agent.Observation{Round: 1, Map: 1, Party: []agent.PartyMon{{Level: 8}}}
	p.prepareRunContext(obs)
	p.prepareRunContext(obs) // same observation as a retry
	if p.strategy.NoProgress != 0 {
		t.Fatalf("same-round retry advanced no-progress counter: %d", p.strategy.NoProgress)
	}

	obs.Round = 2
	p.prepareRunContext(obs)
	if p.strategy.NoProgress != 1 {
		t.Fatalf("next identical round no-progress = %d, want 1", p.strategy.NoProgress)
	}
}

func TestStatsPlannerClearsReplanSignalOnObservableProgress(t *testing.T) {
	inner := &agent.LLMPlanner{ExtraSystem: "baseline"}
	p := newStatsPlanner(inner, nil, nil, nil)

	for round := 1; round <= strategicReplanAfter+1; round++ {
		p.prepareRunContext(agent.Observation{Round: round, Map: 1, Intent: "explore", Party: []agent.PartyMon{{Level: 8}}})
	}
	if !strings.Contains(inner.ExtraSystem, "RUN REPLAN SIGNAL") {
		t.Fatalf("expected replan signal after stall: %q", inner.ExtraSystem)
	}

	p.prepareRunContext(agent.Observation{Round: strategicReplanAfter + 2, Map: 2, Intent: "explore", Party: []agent.PartyMon{{Level: 8}}})
	if inner.ExtraSystem != "baseline" {
		t.Fatalf("progress did not clear temporary signal: %q", inner.ExtraSystem)
	}
	if p.strategy.NoProgress != 0 {
		t.Fatalf("progress did not reset no-progress counter: %d", p.strategy.NoProgress)
	}
}

// The stall RAM capture fires on the EDGE of the replan signal. A stall that
// persists for twenty rounds is one piece of evidence, not twenty 64 KiB
// files, and observable progress re-arms it for the next episode.
func TestStatsPlannerCapturesStallOncePerEpisode(t *testing.T) {
	p := newStatsPlanner(&agent.LLMPlanner{}, nil, nil, nil)
	stalled := func(round int) agent.Observation {
		return agent.Observation{Round: round, Map: 1, Intent: "explore", Party: []agent.PartyMon{{Level: 8}}}
	}

	for round := 1; round <= strategicReplanAfter+3; round++ {
		p.prepareRunContext(stalled(round))
	}
	if !p.stallCaptured {
		t.Fatal("a sustained stall never armed the capture")
	}

	// Observable progress (a new map) clears the signal and re-arms capture.
	p.prepareRunContext(agent.Observation{
		Round: strategicReplanAfter + 4, Map: 2, Intent: "explore",
		Party: []agent.PartyMon{{Level: 8}},
	})
	if p.stallCaptured {
		t.Fatal("progress did not re-arm the stall capture")
	}
}
