package agent

import (
	"strings"
	"testing"
)

// TestChoiceSchemaAsksOnlyForTheIndex: three runs have now died because a
// small model stapled an optional argument onto a choice it did not belong
// to, and at temperature 0 the rejection feedback never changed the answer.
// The menu carries every value already, so the reply asks for a number and
// a sentence, whatever is offered.
func TestChoiceSchemaAsksOnlyForTheIndex(t *testing.T) {
	props, ok := choiceSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties object")
	}
	for _, field := range []string{"level", "species", "item", "quantity", "flee", "slot"} {
		if _, present := props[field]; present {
			t.Errorf("%q is in the reply schema; a model handed the field will attach it to the wrong choice", field)
		}
	}
	for _, field := range []string{"choice", "intent"} {
		if _, present := props[field]; !present {
			t.Errorf("%q missing from the reply schema", field)
		}
	}
	if len(props) != 2 {
		t.Errorf("schema asks for %d fields, want exactly choice and intent", len(props))
	}
}

// TestWithArgsStillRejects: the schema is an optimisation, not the safety
// mechanism. A server that ignores response_format can still send an
// argument, and it must still be refused rather than silently dropped.
func TestWithArgsStillRejects(t *testing.T) {
	level := 7
	potion, _ := ItemByName("potion")
	_, err := WithArgs(Objective{Kind: KindUseItem, Item: potion, Slot: 0}, ReplyArgs{Level: &level})
	if err == nil {
		t.Fatal("a level on a use-item was accepted; it must be rejected")
	}
	if !strings.Contains(err.Error(), "level argument") {
		t.Fatalf("error does not name the misplaced argument: %v", err)
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
