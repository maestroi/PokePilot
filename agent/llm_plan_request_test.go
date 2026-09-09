package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type strategicRoundTrip func(*http.Request) (*http.Response, error)

func (f strategicRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStrategistUsesThinkingModeAndLargeBudget(t *testing.T) {
	var request map[string]any
	client := &http.Client{Transport: strategicRoundTrip(func(r *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		body := `{"model":"test-model","choices":[{"message":{"content":"{\"goal\":\"go north\",\"steps\":[\"go to route 1\"]}"},"finish_reason":"stop"}]}`
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
	})}
	p := &LLMPlanner{BaseURL: "http://unused", Model: "test-model", Client: client, NoThink: true, MaxTokens: 512, ReasoningEffort: "medium"}
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	if _, err := p.Strategize(Observation{Round: 3}, offered, "initial"); err != nil {
		t.Fatalf("Strategize: %v", err)
	}
	if got := int(request["max_tokens"].(float64)); got < strategicReplyTokens {
		t.Fatalf("max_tokens=%d, want >=%d", got, strategicReplyTokens)
	}
	if got := request["reasoning_effort"]; got != "medium" {
		t.Fatalf("reasoning_effort=%v, want medium", got)
	}
	if _, present := request["chat_template_kwargs"]; present {
		t.Fatalf("strategist inherited cheap NoThink request: %+v", request["chat_template_kwargs"])
	}
	rf := request["response_format"].(map[string]any)
	schema := rf["json_schema"].(map[string]any)
	if schema["name"] != "objective_plan" {
		t.Fatalf("schema name=%v, want objective_plan", schema["name"])
	}
}

// TestStrategistReasoningEffortOffDisablesThinking locks in the escape
// hatch added when "low" still reasoned too long on a big real-run prompt:
// "off" must disable thinking via the chat-template argument, the same
// mechanism NoThink gives the chooser, and must not also send
// reasoning_effort (meaningless once thinking is off, and the field this
// server treats as a request to reason more, not less).
func TestStrategistReasoningEffortOffDisablesThinking(t *testing.T) {
	var request map[string]any
	client := &http.Client{Transport: strategicRoundTrip(func(r *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		body := `{"model":"test-model","choices":[{"message":{"content":"{\"goal\":\"go north\",\"steps\":[\"go to route 1\"]}"},"finish_reason":"stop"}]}`
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
	})}
	p := &LLMPlanner{BaseURL: "http://unused", Model: "test-model", Client: client, ReasoningEffort: "off"}
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	if _, err := p.Strategize(Observation{Round: 3}, offered, "initial"); err != nil {
		t.Fatalf("Strategize: %v", err)
	}
	if _, present := request["reasoning_effort"]; present {
		t.Fatalf("reasoning_effort should be omitted when thinking is off, got %v", request["reasoning_effort"])
	}
	ctk, ok := request["chat_template_kwargs"].(map[string]any)
	if !ok {
		t.Fatalf("chat_template_kwargs missing; want enable_thinking:false, got %+v", request["chat_template_kwargs"])
	}
	if enabled, ok := ctk["enable_thinking"].(bool); !ok || enabled {
		t.Fatalf("enable_thinking=%v, want false", ctk["enable_thinking"])
	}
}

// TestStrategistRecoveryReasonEscalatesReasoning locks in the recovery
// tier: a run that is off by default (bounded selection, no derivation
// needed) should still reason for real once something is going wrong —
// isRecoveryReplan's reasons are exactly run.go's evidence of that, not a
// routine plan_exhausted/story_changed replan.
func TestStrategistRecoveryReasonEscalatesReasoning(t *testing.T) {
	for _, reason := range []string{"stagnation", "stuck", "objective_failed", "blackout", "train_retreat"} {
		var request map[string]any
		client := &http.Client{Transport: strategicRoundTrip(func(r *http.Request) (*http.Response, error) {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &request); err != nil {
				t.Fatal(err)
			}
			body := `{"model":"test-model","choices":[{"message":{"content":"{\"goal\":\"go north\",\"steps\":[\"go to route 1\"]}"},"finish_reason":"stop"}]}`
			return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
		})}
		p := &LLMPlanner{BaseURL: "http://unused", Model: "test-model", Client: client, ReasoningEffort: "off", RecoveryReasoningEffort: "medium"}
		offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
		if _, err := p.Strategize(Observation{Round: 3}, offered, reason); err != nil {
			t.Fatalf("reason %q: Strategize: %v", reason, err)
		}
		if got := request["reasoning_effort"]; got != "medium" {
			t.Errorf("reason %q: reasoning_effort=%v, want medium", reason, got)
		}
		if _, present := request["chat_template_kwargs"]; present {
			t.Errorf("reason %q: still disabled thinking despite recovery escalation: %+v", reason, request["chat_template_kwargs"])
		}
	}
}

// TestStrategistNonRecoveryReasonStaysOff is the control: a routine replan
// (plan finished, story advanced) must not accidentally escalate.
func TestStrategistNonRecoveryReasonStaysOff(t *testing.T) {
	for _, reason := range []string{"plan_exhausted", "story_changed", "badge_changed", "new_requirement", "initial"} {
		var request map[string]any
		client := &http.Client{Transport: strategicRoundTrip(func(r *http.Request) (*http.Response, error) {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &request); err != nil {
				t.Fatal(err)
			}
			body := `{"model":"test-model","choices":[{"message":{"content":"{\"goal\":\"go north\",\"steps\":[\"go to route 1\"]}"},"finish_reason":"stop"}]}`
			return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
		})}
		p := &LLMPlanner{BaseURL: "http://unused", Model: "test-model", Client: client, ReasoningEffort: "off", RecoveryReasoningEffort: "medium"}
		offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
		if _, err := p.Strategize(Observation{Round: 3}, offered, reason); err != nil {
			t.Fatalf("reason %q: Strategize: %v", reason, err)
		}
		if _, present := request["reasoning_effort"]; present {
			t.Errorf("reason %q: escalated when it should have stayed off: reasoning_effort=%v", reason, request["reasoning_effort"])
		}
		ctk, ok := request["chat_template_kwargs"].(map[string]any)
		if !ok || ctk["enable_thinking"] != false {
			t.Errorf("reason %q: thinking not disabled: %+v", reason, request["chat_template_kwargs"])
		}
	}
}
