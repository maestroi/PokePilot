package main

import (
	"context"
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/game"
)

type countingDecisionEngine struct {
	calls int
	resp  agent.DecisionResponse
	err   error
}

func (e *countingDecisionEngine) Decide(context.Context, agent.DecisionRequest) (agent.DecisionResponse, error) {
	e.calls++
	return e.resp, e.err
}

func battleShadowTurn() game.BattleDecisionState {
	return game.BattleDecisionState{
		Context:  game.BattleContextWild,
		Active:   game.BattleMon{Species: "squirtle", Level: 14, HP: 38, MaxHP: 40, Types: []string{"water"}},
		Opponent: game.BattleMon{Species: "geodude", Level: 12, HP: 33, MaxHP: 33, Types: []string{"rock", "ground"}},
		Moves: []game.BattleMoveOption{
			{Slot: 0, Move: "tackle", Type: "normal", Power: 35, PP: 30},
			{Slot: 1, Move: "water gun", Type: "water", Power: 40, PP: 20, Effectiveness: 4},
		},
	}
}

func battleShadowPlanner(engine agent.DecisionEngine) *statsPlanner {
	return &statsPlanner{
		decision: agent.DecisionSettings{Engine: engine, Backend: "jev", Battles: true, Shadow: true, MinConfidence: 0.65},
		counts:   map[string]int{},
	}
}

func TestObserveBattleTurnRecordsShadowAgreement(t *testing.T) {
	engine := &countingDecisionEngine{resp: agent.DecisionResponse{Choice: "move:1", Probabilities: map[string]float64{"move:0": 0.1, "move:1": 0.9}}}
	planner := battleShadowPlanner(engine)
	move := func(slot int) game.BattleAction { return game.BattleAction{Kind: game.BattleActionMove, Slot: slot} }

	planner.ObserveBattleTurn(battleShadowTurn(), move(1))
	planner.ObserveBattleTurn(battleShadowTurn(), move(0))

	if engine.calls != 2 || planner.stats.DecisionAgreements != 1 || planner.stats.DecisionDisagreements != 1 {
		t.Fatalf("calls %d agreements %d disagreements %d, want 2/1/1", engine.calls, planner.stats.DecisionAgreements, planner.stats.DecisionDisagreements)
	}
	kind := planner.stats.DecisionSummary.Kinds[agent.DecisionKindBattleTurn]
	if kind == nil || kind.Calls != 2 {
		t.Fatalf("battle_turn summary = %+v, want 2 calls", kind)
	}
	last := planner.stats.DecisionRecords[len(planner.stats.DecisionRecords)-1]
	if !last.Shadow || last.Agreed == nil || *last.Agreed || last.Executed == "" || last.Executed == "move:0" {
		t.Fatalf("last record = %+v, want a labelled shadow disagreement", last)
	}
}

func TestObserveBattleTurnOnlyRunsForShadowBattles(t *testing.T) {
	for name, settings := range map[string]agent.DecisionSettings{
		"battles off": {Battles: false, Shadow: true},
		"active mode": {Battles: true, Shadow: false},
	} {
		engine := &countingDecisionEngine{resp: agent.DecisionResponse{Choice: "move:0", Probabilities: map[string]float64{"move:0": 1}}}
		settings.Engine = engine
		planner := &statsPlanner{decision: settings, counts: map[string]int{}}
		planner.ObserveBattleTurn(battleShadowTurn(), game.BattleAction{Kind: game.BattleActionMove})
		if engine.calls != 0 || planner.stats.DecisionCalls != 0 {
			t.Fatalf("%s: engine called %d times, recorded %d", name, engine.calls, planner.stats.DecisionCalls)
		}
	}
}

func TestObserveBattleTurnSuspendsAfterConsecutiveTransportFailures(t *testing.T) {
	engine := &countingDecisionEngine{err: errors.New("dial tcp: connection refused")}
	planner := battleShadowPlanner(engine)
	for i := 0; i < battleShadowMaxFailures+3; i++ {
		planner.ObserveBattleTurn(battleShadowTurn(), game.BattleAction{Kind: game.BattleActionMove})
	}
	if engine.calls != battleShadowMaxFailures {
		t.Fatalf("engine called %d times, want suspension after %d", engine.calls, battleShadowMaxFailures)
	}
	if planner.stats.DecisionCalls != battleShadowMaxFailures {
		t.Fatalf("recorded %d calls, want each failed call recorded", planner.stats.DecisionCalls)
	}
	if !planner.stats.DecisionBattlesPaused {
		t.Fatal("DecisionBattlesPaused = false, want the pause visible on the run")
	}
	if last := planner.stats.DecisionRecords[len(planner.stats.DecisionRecords)-1]; last.ErrorKind != agent.DecisionErrorBackend {
		t.Fatalf("ErrorKind = %q, want %q", last.ErrorKind, agent.DecisionErrorBackend)
	}
}

func TestReportingPlannerForwardsBattleTurns(t *testing.T) {
	engine := &countingDecisionEngine{resp: agent.DecisionResponse{Choice: "move:0", Probabilities: map[string]float64{"move:0": 1}}}
	var observer agent.BattleTurnObserver = reportingPlanner{inner: battleShadowPlanner(engine)}
	observer.ObserveBattleTurn(battleShadowTurn(), game.BattleAction{Kind: game.BattleActionMove})
	if engine.calls != 1 {
		t.Fatalf("farm planner decorator forwarded %d battle turns, want 1", engine.calls)
	}
}
