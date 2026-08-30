package agent

import "testing"

// TestChoiceSchemaOnlyOffersApplicableArguments is the fix for the round-1
// stall: a model handed an optional field fills it in, and WithArgs then
// rejects the argument as inapplicable, three asks running. A round whose
// menu takes no arguments must ask only for the choice.
func TestChoiceSchemaOnlyOffersApplicableArguments(t *testing.T) {
	props := func(offered []Objective) map[string]any {
		return choiceSchema(offered)["properties"].(map[string]any)
	}

	starters := []Objective{
		{Kind: KindStarter},
		{Kind: KindGoTo, Place: "pallet town"},
	}
	for _, field := range []string{"level", "species", "item", "quantity", "flee"} {
		if _, ok := props(starters)[field]; ok {
			t.Fatalf("%q offered on a menu that cannot take it", field)
		}
	}
	if _, ok := props(starters)["choice"]; !ok {
		t.Fatal("choice must always be askable")
	}

	withArgs := append(starters,
		Objective{Kind: KindTrain, Level: 9},
		Objective{Kind: KindCatch},
		Objective{Kind: KindBuy},
	)
	for _, field := range []string{"level", "species", "item", "quantity"} {
		if _, ok := props(withArgs)[field]; !ok {
			t.Fatalf("%q missing though an objective that takes it is offered", field)
		}
	}
	if _, ok := props(withArgs)["flee"]; ok {
		t.Fatal("flee must never be in the schema; the menu carries both variants")
	}
}

// TestMaxTokensDefaultsAndOverrides: the reply cap is room for a reasoning
// model to think, so it must be raisable without editing code. Zero keeps
// the small-model default.
func TestMaxTokensDefaultsAndOverrides(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_MAX_TOKENS", "4096")
	if got := NewLLMPlanner().MaxTokens; got != 4096 {
		t.Fatalf("MaxTokens = %d, want the POKEPILOT_LLM_MAX_TOKENS value", got)
	}
	t.Setenv("POKEPILOT_LLM_MAX_TOKENS", "not a number")
	if got := NewLLMPlanner().MaxTokens; got != 0 {
		t.Fatalf("MaxTokens = %d on unparseable input, want 0 (the default)", got)
	}
}

// TestNoThinkIsOffByDefault: the request must be byte-identical for
// callers that never asked for it, or every prior measurement is invalid.
func TestNoThinkIsOffByDefault(t *testing.T) {
	if NewLLMPlanner().NoThink {
		t.Fatal("NoThink is on with no environment set")
	}
	t.Setenv("POKEPILOT_LLM_NO_THINK", "0")
	if NewLLMPlanner().NoThink {
		t.Fatal(`POKEPILOT_LLM_NO_THINK=0 turned thinking off`)
	}
	t.Setenv("POKEPILOT_LLM_NO_THINK", "1")
	if !NewLLMPlanner().NoThink {
		t.Fatal(`POKEPILOT_LLM_NO_THINK=1 did not take`)
	}
}
