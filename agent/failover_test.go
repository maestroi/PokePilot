package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestFailoverPlannerFallsBackOnTransportAndPinsFallback(t *testing.T) {
	primaryServer := brokenLLMServer(t)
	defer primaryServer.Close()

	var fallbackCalls atomic.Int32
	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
		writeLLMChoice(t, w, "cpu-4b", 1)
	}))
	defer fallbackServer.Close()

	primary := &LLMPlanner{BaseURL: primaryServer.URL, Model: "gpu-27b", Goal: "badges:1", ExtraSystem: "progress"}
	fallback := &LLMPlanner{BaseURL: fallbackServer.URL, Model: "cpu-4b", Token: "fallback-token", NoThink: true}
	p := NewFailoverPlanner(primary, fallback)
	var calls []LLMCall
	p.OnCall = func(call LLMCall) { calls = append(calls, call) }

	offered := []Objective{{Kind: KindGoTo, Place: "pallet town"}}
	obs := Observation{Round: 1, RoundsLeft: 10}
	got, err := p.Next(obs, offered)
	if err != nil {
		t.Fatalf("Next after failover: %v", err)
	}
	if got.String() != offered[0].String() {
		t.Fatalf("choice = %q, want %q", got, offered[0])
	}
	if route := p.Route(); route.Backend != "fallback" || route.Model != "cpu-4b" || route.Failovers != 1 {
		t.Fatalf("route = %+v, want fallback cpu-4b after one failover", route)
	}
	if len(calls) != 2 {
		t.Fatalf("endpoint calls = %d, want 2 (failed primary + fallback)", len(calls))
	}
	if h := p.Health(); h.Transport != 1 {
		t.Fatalf("aggregate transport = %d, want 1", h.Transport)
	}
	if fallback.Goal != primary.Goal || fallback.ExtraSystem != primary.ExtraSystem {
		t.Fatalf("fallback context = goal %q extra %q, want primary context", fallback.Goal, fallback.ExtraSystem)
	}
	if fallback.Token != "fallback-token" || !fallback.NoThink {
		t.Fatalf("endpoint config was overwritten during context sync: token=%q noThink=%v", fallback.Token, fallback.NoThink)
	}

	obs.Round = 2
	if _, err := p.Next(obs, offered); err != nil {
		t.Fatalf("second Next on pinned fallback: %v", err)
	}
	if fallbackCalls.Load() != 2 {
		t.Fatalf("fallback calls after second round = %d, want 2", fallbackCalls.Load())
	}
	if primary.Health.Transport != 1 {
		t.Fatalf("primary was retried after failover: transport=%d", primary.Health.Transport)
	}
	if route := p.Route(); route.Failovers != 1 || route.Backend != "fallback" {
		t.Fatalf("fallback did not stay pinned: %+v", route)
	}
}

func TestFailoverPlannerDoesNotFailOverModelRejection(t *testing.T) {
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeLLMChoice(t, w, "gpu-27b", 99)
	}))
	defer primaryServer.Close()

	var fallbackCalls atomic.Int32
	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
		writeLLMChoice(t, w, "cpu-4b", 1)
	}))
	defer fallbackServer.Close()

	p := NewFailoverPlanner(
		&LLMPlanner{BaseURL: primaryServer.URL, Model: "gpu-27b"},
		&LLMPlanner{BaseURL: fallbackServer.URL, Model: "cpu-4b"},
	)
	_, err := p.Next(Observation{Round: 1}, []Objective{{Kind: KindGoTo, Place: "pallet town"}})
	if err == nil {
		t.Fatal("out-of-range model choice unexpectedly succeeded")
	}
	if errors.Is(err, ErrTransport) {
		t.Fatalf("model rejection was misclassified as transport: %v", err)
	}
	if fallbackCalls.Load() != 0 {
		t.Fatalf("model rejection triggered %d fallback calls", fallbackCalls.Load())
	}
	if route := p.Route(); route.Failovers != 0 || route.Backend != "primary" {
		t.Fatalf("model rejection changed route: %+v", route)
	}
}

func TestFailoverPlannerTypesTransportFailureWithoutFallback(t *testing.T) {
	primaryServer := brokenLLMServer(t)
	defer primaryServer.Close()

	p := NewFailoverPlanner(&LLMPlanner{BaseURL: primaryServer.URL, Model: "gpu-27b"}, nil)
	_, err := p.Next(Observation{Round: 1}, []Objective{{Kind: KindGoTo, Place: "pallet town"}})
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("transport error = %v, want ErrTransport", err)
	}
	if h := p.Health(); h.Transport != 1 {
		t.Fatalf("transport health = %+v, want one transport failure", h)
	}
}

func TestOptionalLLMConfigFromEnvPreservesFallbackSemantics(t *testing.T) {
	const prefix = "TEST_LLM_FALLBACK_"
	t.Setenv(prefix+"URL", " http://fallback.test/v1 ")
	t.Setenv(prefix+"MODEL", " cpu-4b ")
	t.Setenv(prefix+"TOKEN", "")
	t.Setenv(prefix+"NO_THINK", "0")
	t.Setenv(prefix+"MAX_TOKENS", "2048")
	t.Setenv(prefix+"TIMEOUT", "45s")

	defaults := LLMConfig{
		Model:     "gpu-27b",
		Token:     "primary-token",
		NoThink:   true,
		MaxTokens: 1024,
	}
	got, ok := OptionalLLMConfigFromEnv(prefix, defaults)
	if !ok {
		t.Fatal("configured fallback was not detected")
	}
	if got.BaseURL != "http://fallback.test/v1" || got.Model != "cpu-4b" {
		t.Fatalf("endpoint = %+v", got)
	}
	if got.Token != "" {
		t.Fatalf("explicit empty token = %q, want empty", got.Token)
	}
	if got.NoThink {
		t.Fatal("explicit NO_THINK=0 did not override inherited true")
	}
	if got.MaxTokens != 2048 || got.Timeout != 45*time.Second {
		t.Fatalf("limits = maxTokens %d timeout %s", got.MaxTokens, got.Timeout)
	}
}

func TestOptionalLLMConfigFromEnvNeedsURL(t *testing.T) {
	const prefix = "TEST_LLM_DISABLED_"
	t.Setenv(prefix+"URL", "   ")
	if _, ok := OptionalLLMConfigFromEnv(prefix, LLMConfig{Model: "primary"}); ok {
		t.Fatal("whitespace URL unexpectedly enabled endpoint")
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
	content := "{\"choice\":" + strconv.Itoa(choice) + ",\"intent\":\"continue toward the goal\"}"
	if err := json.NewEncoder(w).Encode(map[string]any{
		"model": model,
		"choices": []any{map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": content,
			},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14},
	}); err != nil {
		t.Errorf("encode reply: %v", err)
	}
}
