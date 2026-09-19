package main

import (
	"context"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

type statsDecisionEngine struct {
	resp agent.DecisionResponse
	err  error
}

func (e statsDecisionEngine) Decide(context.Context, agent.DecisionRequest) (agent.DecisionResponse, error) {
	return e.resp, e.err
}

func TestStatsPlannerTypedObjectiveRecordsDecisionTelemetry(t *testing.T) {
	planner := &statsPlanner{
		inner: &agent.LLMPlanner{Goal: "make progress"},
		decision: agent.DecisionSettings{
			Engine:             statsDecisionEngine{resp: agent.DecisionResponse{Choice: "2", Probabilities: map[string]float64{"1": 0.1, "2": 0.9}, Backend: "typed-test", Model: "tiny"}},
			Backend:            "typed-test",
			ObjectiveSelection: true,
			MinConfidence:      0.7,
		},
		counts: map[string]int{},
	}
	offered := []agent.Objective{
		{Kind: agent.KindGoTo, Place: "pallet town"},
		{Kind: agent.KindHeal},
	}
	got, ok := planner.typedObjective(agent.Observation{Round: 7, RoundsLeft: 3}, offered)
	if !ok || got.String() != offered[1].String() {
		t.Fatalf("typed objective = %s ok=%v, want %s", got, ok, offered[1])
	}
	if planner.stats.DecisionCalls != 1 || planner.stats.DecisionFallbacks != 0 || planner.stats.DecisionConfidence != 0.9 {
		t.Fatalf("decision stats = %#v", planner.stats)
	}
	if planner.stats.Rounds != 1 || planner.stats.FastCalls != 1 || len(planner.stats.Choices) != 1 {
		t.Fatalf("choice stats = rounds %d fast %d choices %#v", planner.stats.Rounds, planner.stats.FastCalls, planner.stats.Choices)
	}
}

func TestStatsPlannerTypedObjectiveFallsBackBelowConfidenceThreshold(t *testing.T) {
	planner := &statsPlanner{
		inner: &agent.LLMPlanner{},
		decision: agent.DecisionSettings{
			Engine:        statsDecisionEngine{resp: agent.DecisionResponse{Choice: "1", Probabilities: map[string]float64{"1": 0.55, "2": 0.45}}},
			MinConfidence: 0.7,
		},
		counts: map[string]int{},
	}
	offered := []agent.Objective{
		{Kind: agent.KindGoTo, Place: "pallet town"},
		{Kind: agent.KindHeal},
	}
	if _, ok := planner.typedObjective(agent.Observation{}, offered); ok {
		t.Fatal("low-confidence typed objective unexpectedly accepted")
	}
	if planner.stats.DecisionCalls != 1 || planner.stats.DecisionRejected != 1 || planner.stats.DecisionFallbacks != 1 {
		t.Fatalf("decision stats = calls %d rejected %d fallbacks %d", planner.stats.DecisionCalls, planner.stats.DecisionRejected, planner.stats.DecisionFallbacks)
	}
}

func TestStatsPlannerFailureDecisionIsIndependentlyConfigurable(t *testing.T) {
	probabilities := map[string]float64{
		"retry": 0.06, "recover": 0.06, "replan": 0.06,
		"pause": 0.70, "impossible": 0.06, "unknown": 0.06,
	}
	planner := &statsPlanner{
		decision: agent.DecisionSettings{
			Engine:          statsDecisionEngine{resp: agent.DecisionResponse{Choice: "pause", Probabilities: probabilities}},
			FailureRecovery: true,
			MinConfidence:   0.65,
		},
		counts: map[string]int{},
	}
	resp, err := planner.DecideFailure(agent.ObjectiveResult{
		Objective: agent.Objective{Kind: agent.KindGoTo, Place: "pewter city"},
		Outcome:   agent.OutcomeBlocked,
	})
	if err != nil {
		t.Fatalf("DecideFailure: %v", err)
	}
	if resp.Choice != "pause" || planner.stats.DecisionKind != agent.DecisionKindFailureRecovery {
		t.Fatalf("response = %#v stats kind = %q", resp, planner.stats.DecisionKind)
	}
}
