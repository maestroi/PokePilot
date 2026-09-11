package main

import (
	"math"
	"testing"
)

func TestSummarizeOutcomesAggregatesLLMUsage(t *testing.T) {
	runs := []statsRun{
		{
			Planner: "llm", LLMProfile: "auto",
			Stats: &statsLLM{
				Calls: 4, Rounds: 3, Rejected: 1, Repeats: 1,
				AvgSeconds: 10, SuccessfulAvgSeconds: 8, RejectedAvgSeconds: 16, StrategicAvgSeconds: 20,
				PromptTokens: 1000, CompletionTokens: 100, StrategicCalls: 1, FastCalls: 3,
				Fallbacks: 1, Model: "qwen-a", ResponseModel: "qwen-a", Backend: "primary",
			},
		},
		{
			Planner: "llm", LLMProfile: "auto",
			Stats: &statsLLM{
				Calls: 2, Rounds: 2,
				AvgSeconds: 5, SuccessfulAvgSeconds: 5,
				PromptTokens: 400, CompletionTokens: 40, FastCalls: 2,
				Model: "qwen-a", ResponseModel: "qwen-a", Backend: "primary",
			},
		},
		{
			Planner: "llm", LLMProfile: "gpu",
			Stats: &statsLLM{
				Calls: 3, Rounds: 1, Rejected: 2,
				AvgSeconds: 15, SuccessfulAvgSeconds: 10, RejectedAvgSeconds: 17.5, StrategicAvgSeconds: 16,
				PromptTokens: 700, CompletionTokens: 70, StrategicCalls: 2, FastCalls: 1,
				Transport: 1, Failovers: 1, Model: "qwen-b", ResponseModel: "qwen-b", Backend: "fallback",
			},
		},
	}

	got := summarizeOutcomes(runs).LLM
	if got.TrackedRuns != 3 || got.Calls != 9 || got.Rounds != 6 || got.Rejected != 3 || got.Repeats != 1 {
		t.Fatalf("counts = %+v", got)
	}
	if got.PromptTokens != 2100 || got.CompletionTokens != 210 {
		t.Fatalf("tokens = %d/%d", got.PromptTokens, got.CompletionTokens)
	}
	if got.Transport != 1 || got.Fallbacks != 1 || got.Failovers != 1 {
		t.Fatalf("reliability = transport %d fallback %d failover %d", got.Transport, got.Fallbacks, got.Failovers)
	}
	if got.FastCalls != 6 || got.StrategicCalls != 3 {
		t.Fatalf("planner mix = fast %d strategic %d", got.FastCalls, got.StrategicCalls)
	}
	assertNear(t, got.AvgSeconds, 95.0/9.0)
	assertNear(t, got.SuccessfulAvgSeconds, 44.0/6.0)
	assertNear(t, got.RejectedAvgSeconds, 17)
	assertNear(t, got.StrategicAvgSeconds, 52.0/3.0)

	if len(got.Profiles) != 2 {
		t.Fatalf("profiles = %+v", got.Profiles)
	}
	if got.Profiles[0].Profile != "auto" || got.Profiles[0].Calls != 6 || got.Profiles[0].Runs != 2 {
		t.Fatalf("auto profile = %+v", got.Profiles[0])
	}
	assertNear(t, got.Profiles[0].AvgSeconds, 50.0/6.0)
	if got.Profiles[1].Profile != "gpu" || got.Profiles[1].Calls != 3 || got.Profiles[1].Rejected != 2 {
		t.Fatalf("gpu profile = %+v", got.Profiles[1])
	}

	if len(got.LatestModels) != 2 {
		t.Fatalf("latest models = %+v", got.LatestModels)
	}
	if got.LatestModels[0].Name != "qwen-a" || got.LatestModels[0].Backend != "primary" || got.LatestModels[0].Runs != 2 {
		t.Fatalf("primary model = %+v", got.LatestModels[0])
	}
}

func assertNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %.9f, want %.9f", got, want)
	}
}
