package main

import (
	"testing"
	"time"

	"github.com/maestroi/pokepilot/agent"
)

func TestStatsPlannerKeepsPerCallTokensSeparateFromCumulativeUsage(t *testing.T) {
	primary := &agent.LLMPlanner{Model: "test-model"}
	s := &statsPlanner{
		router:           agent.NewFailoverPlanner(primary, nil),
		counts:           map[string]int{},
		lastTelemetrySeq: currentLLMTelemetrySeq(),
	}

	primary.Health.PromptTokens = 100
	primary.Health.CompletionTokens = 10
	s.recordCall(agent.LLMCall{Strategic: true, Duration: time.Second})
	if s.stats.PromptTokens != 100 || s.stats.CompletionTokens != 10 {
		t.Fatalf("cumulative usage = %d/%d", s.stats.PromptTokens, s.stats.CompletionTokens)
	}
	if s.stats.LastPromptTokens != 100 || s.stats.LastCompletionTokens != 10 {
		t.Fatalf("first-call usage = %d/%d", s.stats.LastPromptTokens, s.stats.LastCompletionTokens)
	}

	primary.Health.PromptTokens = 250
	primary.Health.CompletionTokens = 25
	s.recordCall(agent.LLMCall{Strategic: true, Duration: time.Second})
	if s.stats.PromptTokens != 250 || s.stats.CompletionTokens != 25 {
		t.Fatalf("cumulative usage = %d/%d", s.stats.PromptTokens, s.stats.CompletionTokens)
	}
	if s.stats.LastPromptTokens != 150 || s.stats.LastCompletionTokens != 15 {
		t.Fatalf("second-call usage = %d/%d, want 150/15", s.stats.LastPromptTokens, s.stats.LastCompletionTokens)
	}
}
