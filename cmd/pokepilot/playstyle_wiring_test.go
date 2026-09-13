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

func TestFarmStatsPlannerConsumesLeasedRunPolicy(t *testing.T) {
	var spec farm.Spec
	if err := json.Unmarshal([]byte(`{"run_id":"style-farm","planner":"llm","play_style":"adventure","risk_tolerance":"cautious","wild_encounters":"fight"}`), &spec); err != nil {
		t.Fatal(err)
	}
	p := newStatsPlanner("", "", "badges:1", nil, nil, &heartbeatSnap{})
	if p.playStyle.Name != agent.PlayStyleAdventure {
		t.Fatalf("leased style = %q, want adventure", p.playStyle.Name)
	}
	if p.riskTolerance != agent.RiskToleranceCautious {
		t.Fatalf("leased risk = %q, want cautious", p.riskTolerance)
	}
	if p.wildEncounters != agent.WildEncountersFight {
		t.Fatalf("leased wild policy = %q, want fight", p.wildEncounters)
	}
}

func TestLegacyFarmSpecResetsToCompatibilityPolicy(t *testing.T) {
	var styled farm.Spec
	if err := json.Unmarshal([]byte(`{"run_id":"style-before","planner":"llm","play_style":"completionist","risk_tolerance":"cautious","wild_encounters":"fight"}`), &styled); err != nil {
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
	if p.riskTolerance != agent.RiskToleranceAggressive {
		t.Fatalf("legacy leased risk = %q, want aggressive", p.riskTolerance)
	}
	if p.wildEncounters != agent.WildEncountersPlanner {
		t.Fatalf("legacy leased wild policy = %q, want planner", p.wildEncounters)
	}
}

func TestLocalRunPolicyFlagsFeedStatsPlanner(t *testing.T) {
	oldStyle := *localPlayStyle
	oldRisk := *localRiskTolerance
	oldWild := *localWildEncounters
	*localPlayStyle = agent.PlayStyleCompletionist
	*localRiskTolerance = agent.RiskToleranceBalanced
	*localWildEncounters = agent.WildEncountersFight
	t.Cleanup(func() {
		*localPlayStyle = oldStyle
		*localRiskTolerance = oldRisk
		*localWildEncounters = oldWild
	})

	p := newStatsPlanner("", "", "badges:1", nil, nil, nil)
	if p.playStyle.Name != agent.PlayStyleCompletionist {
		t.Fatalf("local style = %q, want completionist", p.playStyle.Name)
	}
	if p.riskTolerance != agent.RiskToleranceBalanced {
		t.Fatalf("local risk = %q, want balanced", p.riskTolerance)
	}
	if p.wildEncounters != agent.WildEncountersFight {
		t.Fatalf("local wild policy = %q, want fight", p.wildEncounters)
	}
}
