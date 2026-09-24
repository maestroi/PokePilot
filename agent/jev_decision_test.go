package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJevDecisionEngineReturnsValidatedDistribution(t *testing.T) {
	var seen jevSystemOneRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Fatalf("path = %q, want /v1/systemone", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"decision":{"type":"choice","choice":"b","confidence":0.77,"probabilities":{"a":0.2,"b":0.8}}},"usage":{"input_tokens":13,"output_tokens":3}}`))
	}))
	defer server.Close()

	engine := &JevDecisionEngine{
		BaseURL: server.URL,
		Model:   "jev-latest",
		Token:   "secret",
		Client:  server.Client(),
	}
	req := testDecisionRequest()
	req.State = json.RawMessage(`{"badge":2}`)
	resp, err := DecideChecked(t.Context(), engine, req)
	if err != nil {
		t.Fatalf("DecideChecked: %v", err)
	}
	if resp.Choice != "b" || resp.Confidence != 0.8 {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Backend != "jev" || resp.Model != "jev-1.13.0" || resp.Duration <= 0 {
		t.Fatalf("metadata = backend %q model %q duration %s", resp.Backend, resp.Model, resp.Duration)
	}
	if resp.Usage.PromptTokens != 13 || resp.Usage.CompletionTokens != 3 || resp.Usage.InputBytes == 0 || resp.Usage.OutputBytes == 0 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
	if seen.Model != "jev-latest" {
		t.Fatalf("model = %q, want jev-latest", seen.Model)
	}
	state, ok := seen.State.(map[string]any)
	if !ok || state["badge"] != float64(2) {
		t.Fatalf("state = %#v, want badge=2", seen.State)
	}
	q, ok := seen.Questions[jevDecisionQuestionName]
	if !ok || q.Type != "choice" || q.Criteria["a"] != "alpha" || q.Criteria["b"] != "beta" {
		t.Fatalf("question = %#v", q)
	}
}

func TestJevDecisionEngineRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"jev-latest","answers":{},"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	engine := &JevDecisionEngine{BaseURL: server.URL, Model: "jev-latest", Client: server.Client()}
	_, err := engine.Decide(t.Context(), testDecisionRequest())
	if !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("error = %v, want ErrInvalidDecision", err)
	}
}

func TestJevDecisionEngineRejectsUndeclaredChoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"jev-latest","answers":{"decision":{"type":"choice","choice":"escape","confidence":0.9,"probabilities":{"a":0.1,"b":0.9}}},"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	engine := &JevDecisionEngine{BaseURL: server.URL, Model: "jev-latest", Client: server.Client()}
	_, err := engine.Decide(t.Context(), testDecisionRequest())
	if !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("error = %v, want ErrInvalidDecision", err)
	}
}

func TestJevDecisionEngineTimeoutIsObservable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()

	engine := &JevDecisionEngine{
		BaseURL: server.URL,
		Model:   "jev-latest",
		Client:  server.Client(),
		Timeout: 10 * time.Millisecond,
	}
	resp, err := engine.Decide(context.Background(), testDecisionRequest())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if resp.Backend != "jev" || resp.Duration <= 0 {
		t.Fatalf("metadata = %#v", resp)
	}
}

func TestJevDecisionEngineAuthAndAPIFailuresAreObservable(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, http.StatusText(status), status)
			}))
			defer server.Close()

			engine := &JevDecisionEngine{BaseURL: server.URL, Model: "jev-latest", Client: server.Client()}
			resp, err := engine.Decide(t.Context(), testDecisionRequest())
			if err == nil || !strings.Contains(err.Error(), http.StatusText(status)) {
				t.Fatalf("error = %v, want HTTP %s", err, http.StatusText(status))
			}
			if resp.Backend != "jev" || resp.Model != "jev-latest" || resp.Duration <= 0 {
				t.Fatalf("metadata = %#v", resp)
			}
		})
	}
}
