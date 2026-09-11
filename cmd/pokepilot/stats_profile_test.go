package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestNewStatsPlannerHonorsLLMProfile(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "")
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("POKEPILOT_LLM_GPU_URL", "http://7900.example/v1")
	t.Setenv("POKEPILOT_LLM_GPU_MODEL", "7900-model")
	t.Setenv("POKEPILOT_LLM_4090_URL", "http://4090.example/v1")
	t.Setenv("POKEPILOT_LLM_4090_MODEL", "4090-model")

	s := newStatsPlanner("auto", "", "Earn the Boulder Badge.", nil, nil, nil)
	if s.inner.BaseURL != "http://7900.example/v1" || s.inner.Model != "7900-model" {
		t.Fatalf("auto primary = %s %s, want 7900 endpoint", s.inner.BaseURL, s.inner.Model)
	}
	if s.router.Fallback == nil || s.router.Fallback.BaseURL != "http://lan.example/v1" {
		t.Fatalf("auto fallback = %+v, want lan endpoint", s.router.Fallback)
	}

	s = newStatsPlanner("default", "", "Earn the Boulder Badge.", nil, nil, nil)
	if s.inner.BaseURL != "http://lan.example/v1" || s.router.Fallback != nil {
		t.Fatalf("default = primary %s/%s fallback %v", s.inner.BaseURL, s.inner.Model, s.router.Fallback)
	}

	s = newStatsPlanner("gpu", "", "Earn the Boulder Badge.", nil, nil, nil)
	if s.inner.BaseURL != "http://4090.example/v1" || s.inner.Model != "4090-model" || s.router.Fallback != nil {
		t.Fatalf("gpu/4090 = primary %s/%s fallback %v", s.inner.BaseURL, s.inner.Model, s.router.Fallback)
	}
	if s.inner.Goal != "Earn the Boulder Badge." {
		t.Fatalf("goal = %q", s.inner.Goal)
	}
	if agent.NormalizeLLMProfile("AUTO") != agent.LLMProfileAuto {
		t.Fatal("profile normalization drift")
	}
}
