package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestNewStatsPlannerHonorsLLMProfile(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("POKEPILOT_LLM_FALLBACK_URL", "http://gpu.example/v1")
	t.Setenv("POKEPILOT_LLM_FALLBACK_MODEL", "gpu-model")

	s := newStatsPlanner("auto", "Earn the Boulder Badge.", nil, nil, nil)
	if s.inner.BaseURL != "http://gpu.example/v1" || s.inner.Model != "gpu-model" {
		t.Fatalf("auto primary = %s %s, want gpu endpoint", s.inner.BaseURL, s.inner.Model)
	}
	if s.router.Fallback == nil || s.router.Fallback.BaseURL != "http://lan.example/v1" {
		t.Fatalf("auto fallback = %+v, want lan endpoint", s.router.Fallback)
	}

	s = newStatsPlanner("default", "Earn the Boulder Badge.", nil, nil, nil)
	if s.inner.BaseURL != "http://lan.example/v1" || s.router.Fallback != nil {
		t.Fatalf("default = primary %s/%s fallback %v", s.inner.BaseURL, s.inner.Model, s.router.Fallback)
	}

	s = newStatsPlanner("gpu", "Earn the Boulder Badge.", nil, nil, nil)
	if s.inner.BaseURL != "http://gpu.example/v1" || s.router.Fallback != nil {
		t.Fatalf("gpu = primary %s/%s fallback %v", s.inner.BaseURL, s.inner.Model, s.router.Fallback)
	}
	if s.inner.Goal != "Earn the Boulder Badge." {
		t.Fatalf("goal = %q", s.inner.Goal)
	}
	if agent.NormalizeLLMProfile("AUTO") != agent.LLMProfileAuto {
		t.Fatal("profile normalization drift")
	}
}
