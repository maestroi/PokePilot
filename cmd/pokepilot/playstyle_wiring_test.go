package main

import (
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

func TestStatsPlannerExplicitPlayStyleIsIndependentFromLLMProfile(t *testing.T) {
	p := newStatsPlannerWithPlayStyle("auto", "", agent.PlayStyleTeamBuilder, "badges:1", nil, nil, nil)
	if p.playStyle.Name != agent.PlayStyleTeamBuilder {
		t.Fatalf("play style = %q, want %q", p.playStyle.Name, agent.PlayStyleTeamBuilder)
	}
	// Endpoint profile normalization happens independently; selecting a gameplay
	// policy must not replace or overload the LLM routing field.
	if agent.NormalizeLLMProfile("auto") != "auto" {
		t.Fatal("auto endpoint profile unexpectedly changed")
	}
}

func TestFarmStatsPlannerConsumesLeasedPlayStyle(t *testing.T) {
	var spec farm.Spec
	if err := json.Unmarshal([]byte(`{"run_id":"style-farm","planner":"llm","play_style":"adventure"}`), &spec); err != nil {
		t.Fatal(err)
	}
	p := newStatsPlanner("", "", "badges:1", nil, nil, &heartbeatSnap{})
	if p.playStyle.Name != agent.PlayStyleAdventure {
		t.Fatalf("leased style = %q, want adventure", p.playStyle.Name)
	}
}

func TestLegacyFarmSpecResetsToSpeedrun(t *testing.T) {
	var styled farm.Spec
	if err := json.Unmarshal([]byte(`{"run_id":"style-before","planner":"llm","play_style":"completionist"}`), &styled); err != nil {
		t.Fatal(err)
	}
	var legacy farm.Spec
	if err := json.Unmarshal([]byte(`{"run_id":"style-legacy","planner":"llm"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	p := newStatsPlanner("", "", "badges:1", nil, nil, &heartbeatSnap{})
	if p.playStyle.Name != agent.PlayStyleSpeedrun {
		t.Fatalf("legacy leased style = %q, want speedrun", p.playStyle.Name)
	}
}

func TestLocalPlayStyleFlagFeedsStatsPlanner(t *testing.T) {
	old := *localPlayStyle
	*localPlayStyle = agent.PlayStyleCompletionist
	t.Cleanup(func() { *localPlayStyle = old })

	p := newStatsPlanner("", "", "badges:1", nil, nil, nil)
	if p.playStyle.Name != agent.PlayStyleCompletionist {
		t.Fatalf("local style = %q, want completionist", p.playStyle.Name)
	}
}
