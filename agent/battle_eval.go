package agent

import (
	"context"
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

// BattleEvalCase is one ROM-free battle-turn fixture: a portable turn state
// and the action ids considered acceptable for it.
type BattleEvalCase struct {
	Name   string
	State  game.BattleDecisionState
	Accept []string
}

// BattleEvalStats is backend health across one battle suite.
type BattleEvalStats struct {
	Calls            int
	Rejected         int
	PromptTokens     int
	CompletionTokens int
	Model            string
}

// ValidateBattleEvalCases keeps the corpus itself honest: every state is a
// valid actionable turn and every accepted answer is in its legal set.
func ValidateBattleEvalCases(cases []BattleEvalCase) error {
	seen := map[string]bool{}
	for _, tc := range cases {
		if tc.Name == "" || seen[tc.Name] {
			return fmt.Errorf("battle eval: empty or duplicate case name %q", tc.Name)
		}
		seen[tc.Name] = true
		if err := tc.State.Validate(); err != nil {
			return fmt.Errorf("battle eval %s: %w", tc.Name, err)
		}
		if len(tc.Accept) == 0 {
			return fmt.Errorf("battle eval %s: no accepted action", tc.Name)
		}
		for _, id := range tc.Accept {
			if _, err := tc.State.Legal(id); err != nil {
				return fmt.Errorf("battle eval %s: accepted %w", tc.Name, err)
			}
		}
	}
	return nil
}

// EvaluateBattleDecisions scores a typed backend on battle turns without an
// emulator. Only the resolved legal action id is scored.
func EvaluateBattleDecisions(engine DecisionEngine, cases []BattleEvalCase, minConfidence float64) (EvalReport, BattleEvalStats) {
	report := EvalReport{Cases: len(cases), Results: make([]EvalCaseResult, 0, len(cases))}
	var stats BattleEvalStats
	for _, tc := range cases {
		row := EvalCaseResult{Name: tc.Name, Accepted: append([]string(nil), tc.Accept...)}
		choice, err := decideBattleTurn(engine, tc.State, minConfidence, &stats)
		if err != nil {
			row.Error = err.Error()
		} else {
			row.Choice = choice.ID()
			for _, want := range tc.Accept {
				if row.Choice == want {
					row.Passed = true
					break
				}
			}
		}
		if row.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, row)
	}
	return report, stats
}

func decideBattleTurn(engine DecisionEngine, s game.BattleDecisionState, minConfidence float64, stats *BattleEvalStats) (game.BattleAction, error) {
	req, err := BattleDecisionRequest(s)
	if err != nil {
		stats.Rejected++
		return game.BattleAction{}, err
	}
	stats.Calls++
	resp, err := DecideChecked(context.Background(), engine, req)
	stats.PromptTokens += resp.Usage.PromptTokens
	stats.CompletionTokens += resp.Usage.CompletionTokens
	if resp.Model != "" {
		stats.Model = resp.Model
	}
	if err != nil {
		stats.Rejected++
		return game.BattleAction{}, err
	}
	if minConfidence > 0 && resp.Confidence < minConfidence {
		stats.Rejected++
		return game.BattleAction{}, fmt.Errorf("%w: %.3f < %.3f", ErrDecisionLowConfidence, resp.Confidence, minConfidence)
	}
	action, err := ResolveBattleDecision(s, resp)
	if err != nil {
		stats.Rejected++
	}
	return action, err
}

// CoreBattleEvalCases is the built-in battle corpus. States are written in
// the portable vocabulary, so the suite runs without a ROM.
func CoreBattleEvalCases() []BattleEvalCase {
	squirtle := game.BattleMon{Species: "squirtle", Level: 14, HP: 38, MaxHP: 40, Types: []string{"water"}}
	geodude := game.BattleMon{Species: "geodude", Level: 12, HP: 33, MaxHP: 33, Types: []string{"rock", "ground"}}
	squirtleMoves := []game.BattleMoveOption{
		{Slot: 0, Move: "tackle", Type: "normal", Power: 35, Accuracy: 95, PP: 30, Effectiveness: 0.5},
		{Slot: 1, Move: "tail whip", Type: "normal", Accuracy: 100, PP: 30},
		{Slot: 2, Move: "bubble", Type: "water", Power: 20, Accuracy: 100, PP: 25, Effectiveness: 4},
		{Slot: 3, Move: "water gun", Type: "water", Power: 40, Accuracy: 100, PP: 20, Effectiveness: 4},
	}
	exhausted := append([]game.BattleMoveOption(nil), squirtleMoves...)
	exhausted[3].PP, exhausted[3].Unusable = 0, game.BattleUnusableNoPP

	return []BattleEvalCase{
		{
			Name: "ordinary_four_moves_super_effective",
			State: game.BattleDecisionState{
				Context: game.BattleContextWild, Active: squirtle, Opponent: geodude,
				Moves: squirtleMoves, CanRun: true,
			},
			Accept: []string{"move:3"},
		},
		{
			Name: "exhausted_pp_best_move",
			State: game.BattleDecisionState{
				Context: game.BattleContextTrainer, Active: squirtle, Opponent: geodude,
				Moves: exhausted,
			},
			Accept: []string{"move:2"},
		},
		{
			Name: "disabled_move",
			State: game.BattleDecisionState{
				Context:  game.BattleContextTrainer,
				Active:   game.BattleMon{Species: "charmander", Level: 12, HP: 30, MaxHP: 34, Types: []string{"fire"}},
				Opponent: game.BattleMon{Species: "bulbasaur", Level: 12, HP: 20, MaxHP: 35, Types: []string{"grass", "poison"}},
				Moves: []game.BattleMoveOption{
					{Slot: 0, Move: "scratch", Type: "normal", Power: 40, Accuracy: 100, PP: 30, Effectiveness: 1},
					{Slot: 1, Move: "growl", Type: "normal", Accuracy: 100, PP: 40},
					{Slot: 2, Move: "ember", Type: "fire", Power: 40, Accuracy: 100, PP: 25, Effectiveness: 2, Unusable: game.BattleUnusableDisabled},
				},
			},
			Accept: []string{"move:0"},
		},
		{
			Name: "immune_lead_switch_to_counter",
			State: game.BattleDecisionState{
				Context:  game.BattleContextTrainer,
				Active:   game.BattleMon{Species: "pikachu", Level: 20, HP: 44, MaxHP: 50, Types: []string{"electric"}},
				Opponent: game.BattleMon{Species: "onix", Level: 14, HP: 40, MaxHP: 40, Types: []string{"rock", "ground"}},
				Moves: []game.BattleMoveOption{
					{Slot: 0, Move: "thundershock", Type: "electric", Power: 40, Accuracy: 100, PP: 28},
					{Slot: 1, Move: "growl", Type: "normal", Accuracy: 100, PP: 40},
				},
				Switches: []game.BattleSwitchOption{
					{Slot: 1, BattleMon: game.BattleMon{Species: "wartortle", Level: 18, HP: 52, MaxHP: 52, Types: []string{"water"}}},
					{Slot: 2, BattleMon: game.BattleMon{Species: "rattata", Level: 6, MaxHP: 22, Types: []string{"normal"}}, Unusable: game.BattleUnusableFainted},
				},
			},
			Accept: []string{"switch:1"},
		},
		{
			Name: "outmatched_wild_run",
			State: game.BattleDecisionState{
				Context:  game.BattleContextWild,
				Active:   game.BattleMon{Species: "caterpie", Level: 4, HP: 3, MaxHP: 19, Types: []string{"bug"}},
				Opponent: game.BattleMon{Species: "fearow", Level: 25, HP: 70, MaxHP: 70, Types: []string{"normal", "flying"}},
				Moves: []game.BattleMoveOption{
					{Slot: 0, Move: "tackle", Type: "normal", Power: 35, Accuracy: 95, PP: 30, Effectiveness: 1},
					{Slot: 1, Move: "string shot", Type: "bug", Accuracy: 95, PP: 40},
				},
				CanRun: true,
			},
			Accept: []string{"run"},
		},
		{
			Name: "asleep_lead_cure",
			State: game.BattleDecisionState{
				Context:  game.BattleContextTrainer,
				Active:   game.BattleMon{Species: "ivysaur", Level: 22, HP: 60, MaxHP: 62, Status: "asleep", Types: []string{"grass", "poison"}},
				Opponent: game.BattleMon{Species: "wigglytuff", Level: 20, HP: 80, MaxHP: 80, Types: []string{"normal"}},
				Moves: []game.BattleMoveOption{
					{Slot: 0, Move: "razor leaf", Type: "grass", Power: 55, Accuracy: 95, PP: 25, Effectiveness: 1},
				},
				Switches: []game.BattleSwitchOption{
					{Slot: 1, BattleMon: game.BattleMon{Species: "pidgey", Level: 5, MaxHP: 20}, Unusable: game.BattleUnusableFainted},
				},
				Items: []game.BattleItemOption{{Item: "awakening", Target: 0, Quantity: 1}},
			},
			Accept: []string{"item:awakening:0"},
		},
	}
}
