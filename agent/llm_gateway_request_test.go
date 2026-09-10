package agent

import (
	"encoding/json"
	"testing"
)

func TestGatewayChatRequestAllowsReasoningEffortPassthrough(t *testing.T) {
	for _, model := range []string{gatewayModelAuto, gatewayModelGPU, gatewayModelLAN} {
		data, err := json.Marshal(chatRequest{Model: model, ReasoningEffort: "medium"})
		if err != nil {
			t.Fatalf("marshal %s: %v", model, err)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", model, err)
		}
		params, ok := got["allowed_openai_params"].([]any)
		if !ok || len(params) != 1 || params[0] != "reasoning_effort" {
			t.Fatalf("%s allowed_openai_params=%v, want [reasoning_effort]", model, got["allowed_openai_params"])
		}
		if got["reasoning_effort"] != "medium" {
			t.Fatalf("%s reasoning_effort=%v, want medium", model, got["reasoning_effort"])
		}
	}
}

func TestGatewayChatRequestHonoursAliasOverride(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_AUTO_MODEL", "operator-auto")
	data, err := json.Marshal(chatRequest{Model: "operator-auto", ReasoningEffort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["allowed_openai_params"]; !ok {
		t.Fatalf("gateway alias override did not get request allowlist: %s", data)
	}
}

func TestDirectChatRequestDoesNotExposeLiteLLMControl(t *testing.T) {
	data, err := json.Marshal(chatRequest{Model: "qwen3.8-27b", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["allowed_openai_params"]; ok {
		t.Fatalf("direct request leaked LiteLLM control field: %s", data)
	}
	if got["reasoning_effort"] != "medium" {
		t.Fatalf("direct request lost reasoning_effort: %s", data)
	}
}

func TestGatewayChooserDoesNotAddUnusedAllowlist(t *testing.T) {
	data, err := json.Marshal(chatRequest{Model: gatewayModelAuto})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["allowed_openai_params"]; ok {
		t.Fatalf("chooser request added unused LiteLLM control field: %s", data)
	}
}
