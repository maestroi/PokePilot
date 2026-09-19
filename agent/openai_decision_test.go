package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIDecisionEngineReturnsValidatedDistribution(t *testing.T) {
	var seen map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"tiny","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"choice\":\"b\",\"probabilities\":[{\"choice\":\"a\",\"probability\":0.15},{\"choice\":\"b\",\"probability\":0.85}]}"}}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`))
	}))
	defer server.Close()

	engine := &OpenAIDecisionEngine{
		BaseURL: server.URL + "/v1",
		Model:   "tiny",
		Token:   "secret",
		Client:  server.Client(),
	}
	resp, err := DecideChecked(t.Context(), engine, testDecisionRequest())
	if err != nil {
		t.Fatalf("DecideChecked: %v", err)
	}
	if resp.Choice != "b" || resp.Confidence != 0.85 {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 7 || resp.Usage.InputBytes == 0 || resp.Usage.OutputBytes == 0 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
	format, ok := seen["response_format"].(map[string]any)
	if !ok || format["type"] != "json_schema" {
		t.Fatalf("response_format = %#v, want json_schema", seen["response_format"])
	}
}

func TestOpenAIDecisionEngineRejectsServerChoiceOutsideSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"tiny","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"choice\":\"escape\",\"probabilities\":[{\"choice\":\"a\",\"probability\":0.1},{\"choice\":\"b\",\"probability\":0.9}]}"}}]}`))
	}))
	defer server.Close()

	engine := &OpenAIDecisionEngine{BaseURL: server.URL, Model: "tiny", Client: server.Client()}
	_, err := engine.Decide(t.Context(), testDecisionRequest())
	if !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("error = %v, want ErrInvalidDecision", err)
	}
}
