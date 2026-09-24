package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// oracleBattleEngine answers each battle turn with a fixed action id when it
// is declared, spreading the remaining probability over the other choices.
type oracleBattleEngine struct {
	pick func(DecisionRequest) string
	err  error
	seen []DecisionRequest
}

func (e *oracleBattleEngine) Decide(_ context.Context, req DecisionRequest) (DecisionResponse, error) {
	e.seen = append(e.seen, req)
	if e.err != nil {
		return DecisionResponse{}, e.err
	}
	choice := e.pick(req)
	probs := make(map[string]float64, len(req.Choices))
	for _, c := range req.Choices {
		probs[c.ID] = 0.1
	}
	probs[choice] = 0.9
	return DecisionResponse{Choice: choice, Probabilities: probs, Model: "oracle"}, nil
}

func TestBattleDecisionRequestDeclaresExactlyTheLegalSet(t *testing.T) {
	s := CoreBattleEvalCases()[3].State // immune lead with a fainted bench member
	req, err := BattleDecisionRequest(s)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range req.Choices {
		ids = append(ids, c.ID)
	}
	if want := []string{"move:0", "move:1", "switch:1"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("choices = %v, want %v", ids, want)
	}
	if req.Kind != DecisionKindBattleTurn || req.Choices[2].Label != "switch to wartortle L18 (52/52 hp)" {
		t.Fatalf("request = %+v", req)
	}

	invalid := s
	invalid.CanRun = true // trainer battle
	if _, err := BattleDecisionRequest(invalid); !errors.Is(err, game.ErrInvalidBattleState) {
		t.Fatalf("invalid state err = %v", err)
	}
}

func TestResolveBattleDecisionRejectsUndeclaredAction(t *testing.T) {
	s := CoreBattleEvalCases()[3].State
	if _, err := ResolveBattleDecision(s, DecisionResponse{Choice: "switch:2"}); !errors.Is(err, ErrInvalidDecision) || !errors.Is(err, game.ErrIllegalBattleAction) {
		t.Fatalf("fainted switch err = %v", err)
	}
	// An engine naming an action outside the declared set is stopped by
	// DecideChecked before resolution is even attempted.
	engine := &oracleBattleEngine{pick: func(DecisionRequest) string { return "run" }}
	req, _ := BattleDecisionRequest(s)
	if _, err := DecideChecked(context.Background(), engine, req); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("undeclared run err = %v", err)
	}
}

func testMoveBattle() state.BattleState {
	b := state.BattleState{Kind: state.BattleWild, EnemySpecies: 0xA9, EnemyHP: 20, EnemyMaxHP: 20, ActiveSpecies: 0xB1, ActiveHP: 20, ActiveMaxHP: 20}
	b.Moves = [4]state.Move{{ID: 0x21, PP: 10}, {ID: 0x27, PP: 10}, {ID: 0x91, PP: 0}, {ID: 0x37, PP: 5}}
	return b
}

func TestBattleMoveDeciderDisabledKeepsDeterministicPolicy(t *testing.T) {
	calls := 0
	fallback := func(b state.BattleState) int { calls++; return skill.FirstUsableMove(b) }
	policy := (&BattleMoveDecider{Fallback: fallback}).Policy()
	if got := policy(testMoveBattle()); got != 0 || calls != 1 {
		t.Fatalf("disabled decider slot=%d fallback calls=%d", got, calls)
	}
}

func TestBattleMoveDeciderUsesLegalChoiceOrFallsBack(t *testing.T) {
	b := testMoveBattle()
	engine := &oracleBattleEngine{pick: func(DecisionRequest) string { return "move:3" }}
	d := &BattleMoveDecider{Engine: engine, Fallback: skill.FirstUsableMove}
	if got := d.Policy()(b); got != 3 || d.Calls != 1 || d.Fallbacks != 0 {
		t.Fatalf("slot=%d calls=%d fallbacks=%d", got, d.Calls, d.Fallbacks)
	}
	for _, c := range engine.seen[0].Choices {
		if c.ID == "move:2" || c.ID == "run" {
			t.Fatalf("move-only request declared %s", c.ID)
		}
	}

	for name, d := range map[string]*BattleMoveDecider{
		"transport error": {Engine: &oracleBattleEngine{err: errors.New("down")}},
		"exhausted slot":  {Engine: &oracleBattleEngine{pick: func(DecisionRequest) string { return "move:2" }}},
		"low confidence":  {Engine: &oracleBattleEngine{pick: func(DecisionRequest) string { return "move:3" }}, MinConfidence: 0.99},
	} {
		d.Fallback = skill.FirstUsableMove
		if got := d.Policy()(b); got != 0 || d.Fallbacks != 1 {
			t.Errorf("%s: slot=%d fallbacks=%d, want deterministic slot 0", name, got, d.Fallbacks)
		}
	}
}

func TestCoreBattleEvalCases(t *testing.T) {
	cases := CoreBattleEvalCases()
	if err := ValidateBattleEvalCases(cases); err != nil {
		t.Fatal(err)
	}
	accept := map[string]string{}
	for _, tc := range cases {
		accept[tc.Name] = tc.Accept[0]
	}
	engine := &oracleBattleEngine{}
	i := 0
	engine.pick = func(DecisionRequest) string { id := accept[cases[i].Name]; i++; return id }
	report, stats := EvaluateBattleDecisions(engine, cases, 0.5)
	if report.Passed != len(cases) || stats.Calls != len(cases) || stats.Rejected != 0 || stats.Model != "oracle" {
		t.Fatalf("report=%+v stats=%+v", report, stats)
	}

	broken := append([]BattleEvalCase(nil), cases...)
	broken[0].Accept = []string{"switch:4"}
	if err := ValidateBattleEvalCases(broken); err == nil {
		t.Fatal("corpus with an illegal accepted action validated")
	}
}
