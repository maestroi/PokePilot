package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/maestroi/pokepilot/game"
)

// BattleEvalCase is one ROM-free battle-turn fixture: a portable turn state
// and the action ids considered acceptable for it. Tags make disagreement
// reports useful without pretending every case has one objectively perfect
// action.
type BattleEvalCase struct {
	Name   string
	State  game.BattleDecisionState
	Accept []string
	Tags   []string
}

// BattleEvalResult is one backend answer with the evidence needed to inspect
// disagreements rather than reducing the suite to one pass ratio.
type BattleEvalResult struct {
	Name            string             `json:"name"`
	Passed          bool               `json:"passed"`
	Choice          string             `json:"choice,omitempty"`
	Accepted        []string           `json:"accepted"`
	Tags            []string           `json:"tags,omitempty"`
	Confidence      float64            `json:"confidence,omitempty"`
	DurationSeconds float64            `json:"duration_seconds,omitempty"`
	Probabilities   map[string]float64 `json:"probabilities,omitempty"`
	Disagreement    string             `json:"disagreement,omitempty"`
	Error           string             `json:"error,omitempty"`
}

// BattleEvalReport is the battle-specific report. It intentionally keeps
// every case because disagreements are the useful output of this experiment.
type BattleEvalReport struct {
	Cases   int                `json:"cases"`
	Passed  int                `json:"passed"`
	Failed  int                `json:"failed"`
	Results []BattleEvalResult `json:"results"`
}

func (r BattleEvalReport) Score() float64 {
	if r.Cases == 0 {
		return 0
	}
	return float64(r.Passed) / float64(r.Cases)
}

func (r BattleEvalReport) FailureSummary() []string {
	out := make([]string, 0, r.Failed)
	for _, row := range r.Results {
		if row.Passed {
			continue
		}
		if row.Error != "" {
			out = append(out, fmt.Sprintf("%s [%s]: backend error: %s", row.Name, row.Disagreement, row.Error))
			continue
		}
		out = append(out, fmt.Sprintf("%s [%s]: chose %q at %.2f confidence; accepted %v", row.Name, row.Disagreement, row.Choice, row.Confidence, row.Accepted))
	}
	return out
}

// BattleEvalStats is backend health across one battle suite.
type BattleEvalStats struct {
	Calls            int
	Rejected         int
	Errors           int
	LowConfidence    int
	PromptTokens     int
	CompletionTokens int
	Model            string
	Responses        int
	ConfidenceTotal  float64
	Latencies        []float64
}

func (s BattleEvalStats) AverageConfidence() float64 {
	if s.Responses == 0 {
		return 0
	}
	return s.ConfidenceTotal / float64(s.Responses)
}

func (s BattleEvalStats) ErrorRate() float64 {
	if s.Calls == 0 {
		return 0
	}
	return float64(s.Errors) / float64(s.Calls)
}

func (s BattleEvalStats) RejectionRate() float64 {
	if s.Calls == 0 {
		return 0
	}
	return float64(s.Rejected) / float64(s.Calls)
}

func (s BattleEvalStats) LatencyPercentile(p float64) float64 {
	if len(s.Latencies) == 0 {
		return 0
	}
	values := append([]float64(nil), s.Latencies...)
	sort.Float64s(values)
	rank := int(p*float64(len(values)) + 0.5)
	if rank < 1 {
		rank = 1
	}
	if rank > len(values) {
		rank = len(values)
	}
	return values[rank-1]
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
// emulator. It records confidence, probabilities and latency for each answer,
// and keeps a disagreement category for every miss.
func EvaluateBattleDecisions(engine DecisionEngine, cases []BattleEvalCase, minConfidence float64) (BattleEvalReport, BattleEvalStats) {
	report := BattleEvalReport{Cases: len(cases), Results: make([]BattleEvalResult, 0, len(cases))}
	var stats BattleEvalStats
	for _, tc := range cases {
		row := BattleEvalResult{
			Name:     tc.Name,
			Accepted: append([]string(nil), tc.Accept...),
			Tags:     append([]string(nil), tc.Tags...),
		}
		action, resp, err := decideBattleTurn(engine, tc.State, minConfidence, &stats)
		row.Confidence = resp.Confidence
		row.DurationSeconds = resp.Duration.Seconds()
		row.Probabilities = cloneBattleProbabilities(resp.Probabilities)
		if err != nil {
			row.Error = err.Error()
			row.Disagreement = "backend_error"
		} else {
			row.Choice = action.ID()
			for _, want := range tc.Accept {
				if row.Choice == want {
					row.Passed = true
					break
				}
			}
			if !row.Passed {
				row.Disagreement = battleDisagreement(row.Choice, tc.Accept)
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

func decideBattleTurn(engine DecisionEngine, s game.BattleDecisionState, minConfidence float64, stats *BattleEvalStats) (game.BattleAction, DecisionResponse, error) {
	req, err := BattleDecisionRequest(s)
	if err != nil {
		stats.Rejected++
		return game.BattleAction{}, DecisionResponse{}, err
	}
	stats.Calls++
	resp, err := DecideChecked(context.Background(), engine, req)
	stats.PromptTokens += resp.Usage.PromptTokens
	stats.CompletionTokens += resp.Usage.CompletionTokens
	if resp.Model != "" {
		stats.Model = resp.Model
	}
	if resp.Duration > 0 {
		stats.Latencies = append(stats.Latencies, resp.Duration.Seconds())
	}
	if err != nil {
		stats.Rejected++
		stats.Errors++
		return game.BattleAction{}, resp, err
	}
	stats.Responses++
	stats.ConfidenceTotal += resp.Confidence
	if minConfidence > 0 && resp.Confidence < minConfidence {
		stats.Rejected++
		stats.LowConfidence++
		return game.BattleAction{}, resp, fmt.Errorf("%w: %.3f < %.3f", ErrDecisionLowConfidence, resp.Confidence, minConfidence)
	}
	action, err := ResolveBattleDecision(s, resp)
	if err != nil {
		stats.Rejected++
		stats.Errors++
	}
	return action, resp, err
}

func cloneBattleProbabilities(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func battleActionKind(id string) string {
	action, err := game.ParseBattleAction(id)
	if err != nil {
		return "unknown"
	}
	return string(action.Kind)
}

func battleDisagreement(choice string, accepted []string) string {
	if choice == "" {
		return "no_choice"
	}
	choiceKind := battleActionKind(choice)
	kinds := map[string]bool{}
	for _, want := range accepted {
		kinds[battleActionKind(want)] = true
	}
	if kinds[choiceKind] {
		switch choiceKind {
		case string(game.BattleActionMove):
			return "move_slot"
		case string(game.BattleActionSwitch):
			return "switch_target"
		case string(game.BattleActionItem):
			return "item_choice"
		default:
			return "same_action_kind"
		}
	}
	if len(kinds) == 1 {
		for wantKind := range kinds {
			return choiceKind + "_vs_" + wantKind
		}
	}
	return choiceKind + "_vs_acceptable_set"
}

// BattleShadowSampleVersion is the JSONL row schema emitted by farm runs.
const BattleShadowSampleVersion = 1

// BattleShadowSample is one portable live battle decision. State plus Executed
// is sufficient to replay a different backend without a ROM. The original
// shadow answer is retained only as benchmark evidence, not as training data.
type BattleShadowSample struct {
	Version                 int                         `json:"version"`
	DecisionIndex           int                         `json:"decision_index,omitempty"`
	StateFingerprint        string                      `json:"state_fingerprint,omitempty"`
	State                   game.BattleDecisionState    `json:"state"`
	Executed                string                      `json:"executed"`
	Outcome                 *game.BattleDecisionOutcome `json:"outcome,omitempty"`
	ObservedChoice          string                   `json:"observed_choice,omitempty"`
	ObservedConfidence      float64                  `json:"observed_confidence,omitempty"`
	ObservedProbabilities   map[string]float64       `json:"observed_probabilities,omitempty"`
	ObservedDurationSeconds float64                  `json:"observed_duration_seconds,omitempty"`
	ObservedError           string                   `json:"observed_error,omitempty"`
}

func (s BattleShadowSample) Validate() error {
	if s.Version != BattleShadowSampleVersion {
		return fmt.Errorf("battle shadow sample: version %d, want %d", s.Version, BattleShadowSampleVersion)
	}
	if err := s.State.Validate(); err != nil {
		return fmt.Errorf("battle shadow sample: %w", err)
	}
	if _, err := s.State.Legal(s.Executed); err != nil {
		return fmt.Errorf("battle shadow sample: executed %w", err)
	}
	return nil
}

func WriteBattleShadowCorpus(w io.Writer, samples []BattleShadowSample) error {
	enc := json.NewEncoder(w)
	for i, sample := range samples {
		if err := sample.Validate(); err != nil {
			return fmt.Errorf("battle shadow sample %d: %w", i, err)
		}
		if err := enc.Encode(sample); err != nil {
			return err
		}
	}
	return nil
}

func ReadBattleShadowCorpus(r io.Reader) ([]BattleShadowSample, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var out []BattleShadowSample
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var sample BattleShadowSample
		if err := json.Unmarshal(raw, &sample); err != nil {
			return nil, fmt.Errorf("battle shadow corpus line %d: %w", line, err)
		}
		if err := sample.Validate(); err != nil {
			return nil, fmt.Errorf("battle shadow corpus line %d: %w", line, err)
		}
		out = append(out, sample)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func BattleEvalCasesFromShadow(samples []BattleShadowSample) ([]BattleEvalCase, error) {
	cases := make([]BattleEvalCase, 0, len(samples))
	for i, sample := range samples {
		if err := sample.Validate(); err != nil {
			return nil, fmt.Errorf("battle shadow sample %d: %w", i, err)
		}
		cases = append(cases, BattleEvalCase{
			Name:   fmt.Sprintf("shadow_%04d_%s", i+1, sample.State.Context),
			State:  sample.State,
			Accept: []string{sample.Executed},
			Tags:   []string{"shadow", string(sample.State.Context)},
		})
	}
	return cases, ValidateBattleEvalCases(cases)
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
			Tags:   []string{"obvious", "super_effective"},
		},
		{
			Name: "exhausted_pp_best_move",
			State: game.BattleDecisionState{
				Context: game.BattleContextTrainer, Active: squirtle, Opponent: geodude,
				Moves: exhausted,
			},
			Accept: []string{"move:2"},
			Tags:   []string{"pp", "legality"},
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
			Tags:   []string{"disabled", "legality"},
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
			Tags:   []string{"immunity", "switch"},
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
			Tags:   []string{"wild", "run", "low_hp"},
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
			Tags:   []string{"status", "item"},
		},
		{
			Name: "critical_hp_heal",
			State: game.BattleDecisionState{
				Context:  game.BattleContextTrainer,
				Active:   game.BattleMon{Species: "pikachu", Level: 18, HP: 4, MaxHP: 42, Types: []string{"electric"}},
				Opponent: game.BattleMon{Species: "zubat", Level: 16, HP: 35, MaxHP: 35, Types: []string{"poison", "flying"}},
				Moves: []game.BattleMoveOption{
					{Slot: 0, Move: "thundershock", Type: "electric", Power: 40, Accuracy: 100, PP: 20, Effectiveness: 2},
				},
				Items: []game.BattleItemOption{{Item: "potion", Target: 0, Quantity: 1}},
			},
			Accept: []string{"item:potion:0"},
			Tags:   []string{"low_hp", "heal", "item"},
		},
		{
			Name: "avoid_bad_switch_when_winning",
			State: game.BattleDecisionState{
				Context:  game.BattleContextTrainer,
				Active:   game.BattleMon{Species: "squirtle", Level: 16, HP: 36, MaxHP: 44, Types: []string{"water"}},
				Opponent: geodude,
				Moves: []game.BattleMoveOption{
					{Slot: 0, Move: "water gun", Type: "water", Power: 40, Accuracy: 100, PP: 18, Effectiveness: 4},
				},
				Switches: []game.BattleSwitchOption{
					{Slot: 1, BattleMon: game.BattleMon{Species: "pidgey", Level: 7, HP: 20, MaxHP: 20, Types: []string{"normal", "flying"}}},
				},
			},
			Accept: []string{"move:0"},
			Tags:   []string{"bad_switch", "super_effective"},
		},
		{
			Name: "trainer_battle_never_run",
			State: game.BattleDecisionState{
				Context:  game.BattleContextTrainer,
				Active:   game.BattleMon{Species: "rattata", Level: 12, HP: 24, MaxHP: 30, Types: []string{"normal"}},
				Opponent: game.BattleMon{Species: "pidgey", Level: 11, HP: 20, MaxHP: 28, Types: []string{"normal", "flying"}},
				Moves: []game.BattleMoveOption{
					{Slot: 0, Move: "quick attack", Type: "normal", Power: 40, Accuracy: 100, PP: 25, Effectiveness: 1},
					{Slot: 1, Move: "tail whip", Type: "normal", Accuracy: 100, PP: 30},
				},
			},
			Accept: []string{"move:0"},
			Tags:   []string{"trainer", "run_legality"},
		},
		{
			Name: "ambiguous_equal_attacks",
			State: game.BattleDecisionState{
				Context:  game.BattleContextWild,
				Active:   game.BattleMon{Species: "rattata", Level: 12, HP: 26, MaxHP: 30, Types: []string{"normal"}},
				Opponent: game.BattleMon{Species: "pidgey", Level: 11, HP: 24, MaxHP: 28, Types: []string{"normal", "flying"}},
				Moves: []game.BattleMoveOption{
					{Slot: 0, Move: "tackle", Type: "normal", Power: 40, Accuracy: 100, PP: 25, Effectiveness: 1},
					{Slot: 1, Move: "quick attack", Type: "normal", Power: 40, Accuracy: 100, PP: 25, Effectiveness: 1},
				},
				CanRun: true,
			},
			Accept: []string{"move:0", "move:1"},
			Tags:   []string{"ambiguous", "confidence"},
		},
	}
}
