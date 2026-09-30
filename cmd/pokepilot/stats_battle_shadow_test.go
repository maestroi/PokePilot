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

	firstTurn := battleShadowTurn()
	secondTurn := battleShadowTurn()
	secondTurn.Active.HP = 31
	secondTurn.Opponent.HP = 18

	planner.ObserveBattleTurn(firstTurn, move(1))
	planner.ObserveBattleTurn(secondTurn, move(0))
	planner.ObserveBattleResult(game.BattleWon)

	if engine.calls != 2 || planner.stats.DecisionAgreements != 1 || planner.stats.DecisionDisagreements != 1 {
		t.Fatalf("calls %d agreements %d disagreements %d, want 2/1/1", engine.calls, planner.stats.DecisionAgreements, planner.stats.DecisionDisagreements)
	}
	kind := planner.stats.DecisionSummary.Kinds[agent.DecisionKindBattleTurn]
	if kind == nil || kind.Calls != 2 {
		t.Fatalf("battle_turn summary = %+v, want 2 calls", kind)
	}
	if len(planner.stats.DecisionRecords) != 2 {
		t.Fatalf("decision records = %d, want 2", len(planner.stats.DecisionRecords))
	}
	first, last := planner.stats.DecisionRecords[0], planner.stats.DecisionRecords[1]
	if first.DecisionIndex != 1 || first.StateFingerprint == "" || first.BattleOutcome == nil || first.BattleOutcome.Kind != "next_turn" ||
		first.BattleOutcome.ActiveHP != 31 || first.BattleOutcome.OpponentHP != 18 {
		t.Fatalf("first record = %+v, want indexed/fingerprinted next-turn outcome", first)
	}
	if !last.Shadow || last.DecisionIndex != 2 || last.StateFingerprint == "" || last.Agreed == nil || *last.Agreed ||
		last.Executed == "" || last.Executed == "move:0" || last.BattleOutcome == nil ||
		last.BattleOutcome.Kind != "battle_result" || last.BattleOutcome.Result != "won" {
		t.Fatalf("last record = %+v, want a labelled terminal shadow disagreement", last)
	}
	if len(planner.battleShadowSamples) != 2 || planner.battleShadowSamples[0].Outcome == nil ||
		planner.battleShadowSamples[1].Outcome == nil || planner.battleShadowSamples[1].Outcome.Result != "won" {
		t.Fatalf("shadow samples = %+v, want outcomes on both rows", planner.battleShadowSamples)
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

func battleActivePlanner(engine agent.DecisionEngine) *statsPlanner {
	return &statsPlanner{
		decision: agent.DecisionSettings{Engine: engine, Backend: "jev", Battles: true, MinConfidence: 0.65},
		counts:   map[string]int{},
	}
}

func TestDecideBattleMoveActiveControlsOnlyAcceptedConfidentLegalMove(t *testing.T) {
	engine := &countingDecisionEngine{resp: agent.DecisionResponse{
		Choice: "move:1", Confidence: 0.9,
		Probabilities: map[string]float64{"move:0": 0.1, "move:1": 0.9},
	}}
	planner := battleActivePlanner(engine)
	fallback := game.BattleAction{Kind: game.BattleActionMove, Slot: 0}

	got := planner.DecideBattleMove(battleShadowTurn(), fallback)
	if got != (game.BattleAction{Kind: game.BattleActionMove, Slot: 1}) {
		t.Fatalf("active choice = %+v, want move:1", got)
	}
	// The same-turn observer must not complete the active row as if it were the
	// next turn.
	planner.ObserveBattleTurn(battleShadowTurn(), got)
	planner.ObserveBattleResult(game.BattleWon)

	if engine.calls != 1 || planner.stats.DecisionCalls != 1 {
		t.Fatalf("engine calls=%d records=%d, want 1/1", engine.calls, planner.stats.DecisionCalls)
	}
	rec := planner.stats.DecisionRecords[0]
	if rec.Shadow || !rec.Controlled || rec.Fallback || rec.DecisionIndex != 1 || rec.StateFingerprint == "" ||
		rec.Executed == "" || rec.BattleOutcome == nil || rec.BattleOutcome.Result != "won" {
		t.Fatalf("active record = %+v", rec)
	}
	kind := planner.stats.DecisionSummary.Kinds[agent.DecisionKindBattleTurn]
	if kind == nil || kind.Calls != 1 || kind.Controlled != 1 || kind.Fallbacks != 0 {
		t.Fatalf("active summary = %+v", kind)
	}
}

func TestDecideBattleMoveActiveFallsBackOnLowConfidenceAndIllegalChoice(t *testing.T) {
	fallback := game.BattleAction{Kind: game.BattleActionMove, Slot: 0}
	for name, response := range map[string]agent.DecisionResponse{
		"low confidence": {
			Choice: "move:1", Confidence: 0.55,
			Probabilities: map[string]float64{"move:0": 0.45, "move:1": 0.55},
		},
		"illegal": {
			Choice: "move:99", Confidence: 0.99,
			Probabilities: map[string]float64{"move:99": 1},
		},
	} {
		t.Run(name, func(t *testing.T) {
			planner := battleActivePlanner(&countingDecisionEngine{resp: response})
			if got := planner.DecideBattleMove(battleShadowTurn(), fallback); got != fallback {
				t.Fatalf("choice = %+v, want deterministic fallback %+v", got, fallback)
			}
			rec := planner.stats.DecisionRecords[0]
			if rec.Controlled || !rec.Fallback || rec.Executed == "" || rec.Error == "" {
				t.Fatalf("fallback record = %+v", rec)
			}
			kind := planner.stats.DecisionSummary.Kinds[agent.DecisionKindBattleTurn]
			if kind == nil || kind.Controlled != 0 || kind.Fallbacks != 1 {
				t.Fatalf("fallback summary = %+v", kind)
			}
		})
	}
}

func TestDecideBattleMoveKillSwitchLeavesDeterministicPolicyUntouched(t *testing.T) {
	engine := &countingDecisionEngine{resp: agent.DecisionResponse{
		Choice: "move:1", Confidence: 0.9,
		Probabilities: map[string]float64{"move:0": 0.1, "move:1": 0.9},
	}}
	planner := battleActivePlanner(engine)
	planner.decision.Battles = false
	fallback := game.BattleAction{Kind: game.BattleActionMove, Slot: 0}
	if got := planner.DecideBattleMove(battleShadowTurn(), fallback); got != fallback {
		t.Fatalf("disabled battle control = %+v, want %+v", got, fallback)
	}
	if engine.calls != 0 || planner.stats.DecisionCalls != 0 {
		t.Fatalf("disabled control called engine=%d recorded=%d", engine.calls, planner.stats.DecisionCalls)
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
}

func TestReportingPlannerForwardsBattleTurns(t *testing.T) {
	engine := &countingDecisionEngine{resp: agent.DecisionResponse{Choice: "move:0", Probabilities: map[string]float64{"move:0": 1}}}
	inner := battleShadowPlanner(engine)
	decorator := reportingPlanner{inner: inner}
	var observer agent.BattleTurnObserver = decorator
	observer.ObserveBattleTurn(battleShadowTurn(), game.BattleAction{Kind: game.BattleActionMove})
	var outcomes agent.BattleOutcomeObserver = decorator
	outcomes.ObserveBattleResult(game.BattleWon)
	if engine.calls != 1 {
		t.Fatalf("farm planner decorator forwarded %d battle turns, want 1", engine.calls)
	}
	last := inner.stats.DecisionRecords[len(inner.stats.DecisionRecords)-1]
	if last.BattleOutcome == nil || last.BattleOutcome.Result != "won" {
		t.Fatalf("forwarded battle result = %+v", last.BattleOutcome)
	}
}
