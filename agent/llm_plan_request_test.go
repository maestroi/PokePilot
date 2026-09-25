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
	p := &LLMPlanner{BaseURL: "http://unused", Model: "test-model", Client: client, MaxTokens: 1024, ReasoningEffort: "off"}
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	if _, err := p.Strategize(Observation{Round: 3}, offered, "initial"); err != nil {
		t.Fatalf("Strategize: %v", err)
	}
	if _, present := request["reasoning_effort"]; present {
		t.Fatalf("reasoning_effort should be omitted when thinking is off, got %v", request["reasoning_effort"])
	}
	if got := int(request["max_tokens"].(float64)); got != 1024 {
		t.Fatalf("max_tokens=%d, want configured no-thinking cap 1024", got)
	}
	ctk, ok := request["chat_template_kwargs"].(map[string]any)
	if !ok {
		t.Fatalf("chat_template_kwargs missing; want enable_thinking:false, got %+v", request["chat_template_kwargs"])
	}
	if enabled, ok := ctk["enable_thinking"].(bool); !ok || enabled {
		t.Fatalf("enable_thinking=%v, want false", ctk["enable_thinking"])
	}
	// LiteLLM's openai/ provider uses the OpenAI SDK, which strips unknown
	// top-level fields even when allowed_openai_params lists them. extra_body
	// is merged into the upstream llama.cpp JSON, so thinking-off must live
	// there as well as at the top level (direct llama.cpp honours the latter).
	extra, ok := request["extra_body"].(map[string]any)
	if !ok {
		t.Fatalf("extra_body missing; LiteLLM will drop chat_template_kwargs, got %+v", request)
	}
	extraCTK, ok := extra["chat_template_kwargs"].(map[string]any)
	if !ok || extraCTK["enable_thinking"] != false {
		t.Fatalf("extra_body.chat_template_kwargs=%v, want enable_thinking:false", extra["chat_template_kwargs"])
	}
}

func TestStrategistReasoningOffKeepsConfiguredBudgetOnRetry(t *testing.T) {
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
	p := &LLMPlanner{BaseURL: "http://unused", Model: "test-model", Client: client, MaxTokens: 1024, ReasoningEffort: "off"}
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	if _, err := p.StrategizeRetry(Observation{Round: 3}, offered, "blackout", Retry{MaxTokensFactor: 4}); err != nil {
		t.Fatalf("StrategizeRetry: %v", err)
	}
	if got := int(request["max_tokens"].(float64)); got != 1024 {
		t.Fatalf("retry max_tokens=%d, want configured no-thinking cap 1024", got)
	}
}

// TestStrategistRecoveryReasonCannotOverrideOff locks in off as a hard run
// policy. Recovery evidence may request a fresh plan, but it must not silently
// turn thinking back on or raise the configured completion budget.
func TestStrategistRecoveryReasonCannotOverrideOff(t *testing.T) {
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
		p := &LLMPlanner{BaseURL: "http://unused", Model: "test-model", Client: client, MaxTokens: 1024, ReasoningEffort: "off", RecoveryReasoningEffort: "medium"}
		offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
		if _, err := p.Strategize(Observation{Round: 3}, offered, reason); err != nil {
			t.Fatalf("reason %q: Strategize: %v", reason, err)
		}
		if _, present := request["reasoning_effort"]; present {
			t.Errorf("reason %q: reasoning_effort must stay omitted when off, got %v", reason, request["reasoning_effort"])
		}
		ctk, ok := request["chat_template_kwargs"].(map[string]any)
		if !ok || ctk["enable_thinking"] != false {
			t.Errorf("reason %q: thinking not disabled: %+v", reason, request["chat_template_kwargs"])
		}
		if got := int(request["max_tokens"].(float64)); got != 1024 {
			t.Errorf("reason %q: max_tokens=%d, want configured no-thinking cap 1024", reason, got)
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
