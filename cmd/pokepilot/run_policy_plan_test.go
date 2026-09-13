package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestBalancedPlanStopsAfterTrainerForFreshRiskCheck(t *testing.T) {
	p := &statsPlanner{
		riskTolerance:  agent.RiskToleranceBalanced,
		wildEncounters: agent.WildEncountersPlanner,
	}
	offered := []agent.Objective{
		{Kind: agent.KindTrainer, X: 3, Y: 4},
		{Kind: agent.KindTrainer, X: 7, Y: 8},
	}
	plan := agent.Plan{
		Goal: "clear the route",
		Steps: []string{
			offered[0].String(),
			offered[1].String(),
		},
	}
	got := p.boundRiskPlan(plan, offered)
	if len(got.Steps) != 1 || got.Steps[0] != offered[0].String() {
		t.Fatalf("balanced plan = %#v, want stop after first trainer", got.Steps)
	}
}

func TestAggressivePlanKeepsTrainerChain(t *testing.T) {
	p := &statsPlanner{
		riskTolerance:  agent.RiskToleranceAggressive,
		wildEncounters: agent.WildEncountersPlanner,
	}
	offered := []agent.Objective{
		{Kind: agent.KindTrainer, X: 3, Y: 4},
		{Kind: agent.KindTrainer, X: 7, Y: 8},
	}
	plan := agent.Plan{Goal: "clear the route", Steps: []string{offered[0].String(), offered[1].String()}}
	got := p.boundRiskPlan(plan, offered)
	if len(got.Steps) != 2 {
		t.Fatalf("aggressive plan was shortened: %#v", got.Steps)
	}
}

func TestFightEverythingPlanStopsAfterTravelForFreshRiskCheck(t *testing.T) {
	p := &statsPlanner{
		riskTolerance:  agent.RiskToleranceCautious,
		wildEncounters: agent.WildEncountersFight,
	}
	offered := []agent.Objective{
		{Kind: agent.KindGoTo, Place: "route 3"},
		{Kind: agent.KindTrainer, X: 7, Y: 8},
	}
	plan := agent.Plan{Goal: "advance", Steps: []string{offered[0].String(), offered[1].String()}}
	got := p.boundRiskPlan(plan, offered)
	if len(got.Steps) != 1 || got.Steps[0] != offered[0].String() {
		t.Fatalf("fight-everything plan = %#v, want risk check after travel", got.Steps)
	}
}
