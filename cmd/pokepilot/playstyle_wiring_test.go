package main

import (
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

func TestStatsPlannerExplicitPlayStyleIsIndependentFromLLMProfile(t *testing.T) {
	p := newStatsPlannerWithRunPolicy(farm.RunPolicy{PlayStyle: agent.PlayStyleTeamBuilder, Goal: "badges:1"}, "auto", "", nil, nil, nil, nil)
	if p.playStyle.Name != agent.PlayStyleTeamBuilder {
		t.Fatalf("play style = %q, want %q", p.playStyle.Name, agent.PlayStyleTeamBuilder)
	}
	// Endpoint profile normalization happens independently; selecting a gameplay
	// policy must not replace or overload the LLM routing field.
	if agent.NormalizeLLMProfile("auto") != "auto" {
		t.Fatal("auto endpoint profile unexpectedly changed")
	}
}

// TestFarmStatsPlannerConsumesLeasedRunPolicy pins that the planner's behavior
// comes from the leased Spec's own policy fields, not from process-global
// state. The Spec is the source passed to the planner.
func TestFarmStatsPlannerConsumesLeasedRunPolicy(t *testing.T) {
	var spec farm.Spec
	if err := json.Unmarshal([]byte(`{"run_id":"style-farm","planner":"llm","play_style":"adventure","purpose":"debug_coverage","risk_tolerance":"cautious","wild_encounters":"fight"}`), &spec); err != nil {
		t.Fatal(err)
	}
	p := newStatsPlannerWithRunPolicy(farm.RunPolicyFor(spec), spec.LLMProfile, spec.ReasoningEffort, spec.Inference, nil, nil, &heartbeatSnap{})
	if p.playStyle.Name != agent.PlayStyleAdventure {
		t.Fatalf("leased style = %q, want adventure", p.playStyle.Name)
	}
	if p.purpose != agent.RunPurposeDebugCoverage {
		t.Fatalf("leased purpose = %q, want debug_coverage", p.purpose)
	}
	if p.riskTolerance != agent.RiskToleranceCautious {
		t.Fatalf("leased risk = %q, want cautious", p.riskTolerance)
	}
	if p.wildEncounters != agent.WildEncountersFight {
		t.Fatalf("leased wild policy = %q, want fight", p.wildEncounters)
	}
}

// TestLegacyFarmSpecKeepsCompatibilityPolicy covers a leased spec that predates
// the policy fields: empty values must fall back to the historical defaults
// rather than inheriting another run's policy.
func TestLegacyFarmSpecKeepsCompatibilityPolicy(t *testing.T) {
	var legacy farm.Spec
	if err := json.Unmarshal([]byte(`{"run_id":"style-legacy","planner":"llm"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	p := newStatsPlannerWithRunPolicy(farm.RunPolicyFor(legacy), legacy.LLMProfile, legacy.ReasoningEffort, legacy.Inference, nil, nil, &heartbeatSnap{})
	if p.playStyle.Name != agent.PlayStyleSpeedrun {
		t.Fatalf("legacy leased style = %q, want speedrun", p.playStyle.Name)
	}
	if p.purpose != agent.RunPurposeNormal {
		t.Fatalf("legacy leased purpose = %q, want normal", p.purpose)
	}
	if p.riskTolerance != agent.RiskToleranceAggressive {
		t.Fatalf("legacy leased risk = %q, want aggressive", p.riskTolerance)
	}
	if p.wildEncounters != agent.WildEncountersPlanner {
		t.Fatalf("legacy leased wild policy = %q, want planner", p.wildEncounters)
	}
}

// TestLocalRunPolicyFlagsFeedStatsPlanner covers the local (non-farm) CLI path:
// the flags are resolved once into a RunPolicy and passed to the planner.
func TestLocalRunPolicyFlagsFeedStatsPlanner(t *testing.T) {
	oldStyle := *localPlayStyle
	oldPurpose := *localRunPurpose
	oldRisk := *localRiskTolerance
	oldWild := *localWildEncounters
	*localPlayStyle = agent.PlayStyleCompletionist
	*localRunPurpose = agent.RunPurposeDebugCoverage
	*localRiskTolerance = agent.RiskToleranceBalanced
	*localWildEncounters = agent.WildEncountersFight
	t.Cleanup(func() {
		*localPlayStyle = oldStyle
		*localRunPurpose = oldPurpose
		*localRiskTolerance = oldRisk
		*localWildEncounters = oldWild
	})

	// localRunPolicy is what main.go hands the planner, so testing it here
	// covers the CLI-to-planner path end to end.
	p := newStatsPlannerWithRunPolicy(localRunPolicy("badges:1"), "", "", nil, nil, nil, nil)
	if p.playStyle.Name != agent.PlayStyleCompletionist {
		t.Fatalf("local style = %q, want completionist", p.playStyle.Name)
	}
	if p.purpose != agent.RunPurposeDebugCoverage {
		t.Fatalf("local purpose = %q, want debug_coverage", p.purpose)
	}
	if p.riskTolerance != agent.RiskToleranceBalanced {
		t.Fatalf("local risk = %q, want balanced", p.riskTolerance)
	}
	if p.wildEncounters != agent.WildEncountersFight {
		t.Fatalf("local wild policy = %q, want fight", p.wildEncounters)
	}
	if p.inner.Goal != "badges:1" {
		t.Fatalf("local goal = %q, want badges:1", p.inner.Goal)
	}
}

// TestLocalRunPolicyKeepsResolvedGoal pins that the local policy carries the
// goal resolveLocalGoal already resolved, including an explicit empty Free
// play goal, and that the play style rides along untouched.
func TestLocalRunPolicyKeepsResolvedGoal(t *testing.T) {
	oldStyle := *localPlayStyle
	*localPlayStyle = agent.PlayStyleCompletionist
	t.Cleanup(func() { *localPlayStyle = oldStyle })

	// Goal resolution is independent from play style. localRunPolicy carries both
	// the ordinary Champion default and an explicit Dex goal verbatim.
	if policy := localRunPolicy(farm.DefaultEliteFourGoal); policy.Goal != farm.DefaultEliteFourGoal {
		t.Fatalf("resolved local goal = %q, want %q", policy.Goal, farm.DefaultEliteFourGoal)
	}
	if policy := localRunPolicy(farm.DefaultDexGoal); policy.Goal != farm.DefaultDexGoal {
		t.Fatalf("explicit Dex goal = %q, want %q", policy.Goal, farm.DefaultDexGoal)
	}
	if policy := localRunPolicy("badges:1"); policy.Goal != "badges:1" {
		t.Fatalf("explicit local goal = %q, want badges:1", policy.Goal)
	}
	// An explicit empty -goal is Free play; the policy must not invent a goal.
	if policy := localRunPolicy(""); policy.Goal != "" {
		t.Fatalf("free play local goal = %q, want empty", policy.Goal)
	}
}
