package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestStatsPlannerFallsBackOnTransportAndPinsFallback(t *testing.T) {
	primary := brokenLLMServer(t)
	defer primary.Close()

	var fallbackCalls atomic.Int32
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
		writeLLMChoice(t, w, "cpu-4b", 1)
	}))
	defer fallback.Close()

	t.Setenv("POKEPILOT_LLM_FALLBACK_URL", fallback.URL)
	t.Setenv("POKEPILOT_LLM_FALLBACK_MODEL", "cpu-4b")
	t.Setenv("POKEPILOT_LLM_FALLBACK_TOKEN", "")

	inner := &agent.LLMPlanner{BaseURL: primary.URL, Model: "gpu-27b"}
	p := newStatsPlanner(inner, nil, nil)
	offered := []agent.Objective{{Kind: agent.KindGoTo, Place: "pallet town"}}
	obs := agent.Observation{Round: 1, RoundsLeft: 10}

	got, err := p.Next(obs, offered)
	if err != nil {
		t.Fatalf("Next after failover: %v", err)
	}
	if got.String() != offered[0].String() {
		t.Fatalf("choice = %q, want %q", got, offered[0])
	}
	if p.stats.Backend != "fallback" || p.stats.Model != "cpu-4b" || p.stats.Failovers != 1 {
		t.Fatalf("route stats = backend %q model %q failovers %d", p.stats.Backend, p.stats.Model, p.stats.Failovers)
	}
	if p.stats.Calls != 2 || p.stats.Transport != 1 {
		t.Fatalf("calls/transport = %d/%d, want 2/1", p.stats.Calls, p.stats.Transport)
	}
	if fallbackCalls.Load() != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallbackCalls.Load())
	}

	// Once the primary transport has failed, do not probe it every round.
	obs.Round = 2
	if _, err := p.Next(obs, offered); err != nil {
		t.Fatalf("second Next on pinned fallback: %v", err)
	}
	if fallbackCalls.Load() != 2 {
		t.Fatalf("fallback calls after second round = %d, want 2", fallbackCalls.Load())
	}
	if inner.Health.Transport != 1 {
		t.Fatalf("primary was retried after failover: transport=%d", inner.Health.Transport)
	}
	if p.stats.Failovers != 1 || p.stats.Backend != "fallback" {
		t.Fatalf("fallback did not stay pinned: %+v", p.stats)
	}
}

func TestStatsPlannerDoesNotFailOverModelRejection(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeLLMChoice(t, w, "gpu-27b", 99)
	}))
	defer primary.Close()

	var fallbackCalls atomic.Int32
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
		writeLLMChoice(t, w, "cpu-4b", 1)
	}))
	defer fallback.Close()

	t.Setenv("POKEPILOT_LLM_FALLBACK_URL", fallback.URL)
	t.Setenv("POKEPILOT_LLM_FALLBACK_MODEL", "cpu-4b")

	p := newStatsPlanner(&agent.LLMPlanner{BaseURL: primary.URL, Model: "gpu-27b"}, nil, nil)
	_, err := p.Next(agent.Observation{Round: 1}, []agent.Objective{{Kind: agent.KindGoTo, Place: "pallet town"}})
	if err == nil {
		t.Fatal("out-of-range model choice unexpectedly succeeded")
	}
	if errors.Is(err, agent.ErrTransport) {
		t.Fatalf("model rejection was misclassified as transport: %v", err)
	}
	if fallbackCalls.Load() != 0 || p.stats.Failovers != 0 || p.stats.Backend != "primary" {
		t.Fatalf("model rejection triggered failover: calls=%d stats=%+v", fallbackCalls.Load(), p.stats)
	}
}

func TestStatsPlannerTypesTransportFailureWithoutFallback(t *testing.T) {
	primary := brokenLLMServer(t)
	defer primary.Close()
	t.Setenv("POKEPILOT_LLM_FALLBACK_URL", "")

	p := newStatsPlanner(&agent.LLMPlanner{BaseURL: primary.URL, Model: "gpu-27b"}, nil, nil)
	_, err := p.Next(agent.Observation{Round: 1}, []agent.Objective{{Kind: agent.KindGoTo, Place: "pallet town"}})
	if !errors.Is(err, agent.ErrTransport) {
		t.Fatalf("transport error = %v, want ErrTransport", err)
	}
	if p.stats.Calls != 1 || p.stats.Transport != 1 || p.stats.Failovers != 0 {
		t.Fatalf("stats after transport failure = %+v", p.stats)
	}
}

func brokenLLMServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, ok := w.(http.Hijacker)
		if !ok {
			t.Error("httptest ResponseWriter does not support hijacking")
			return
		}
		conn, _, err := h.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		_ = conn.Close()
	}))
}

func writeLLMChoice(t *testing.T, w http.ResponseWriter, model string, choice int) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"model": model,
		"choices": []any{map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": `{"choice":1}`,
			},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14},
	}); err != nil {
		t.Errorf("encode reply: %v", err)
	}
}

func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
