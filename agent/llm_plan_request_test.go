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
