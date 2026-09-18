package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
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

func TestFarmStatsPlannerHonorsLeasedInferenceIdentity(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "")
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("POKEPILOT_LLM_GPU_URL", "http://stale-7900.example/v1")
	t.Setenv("POKEPILOT_LLM_GPU_MODEL", "stale-27b")
	t.Setenv("DYNAMIC_ENDPOINT_TOKEN", "secret")

	identity := &farm.InferenceIdentity{
		DeploymentID: "farm-7900",
		Endpoint: "http://7900.example/v1",
		APIModel: "qwen3.5-9b",
		ModelID: "qwen3.5-9b",
		Compute: "RX 7900 XTX",
		EndpointTokenEnv: "DYNAMIC_ENDPOINT_TOKEN",
	}
	s := newStatsPlannerWithRunPolicyAndInference(
		"auto", "", "speedrunner", "", "", "Earn the Boulder Badge.",
		identity, nil, nil, nil,
	)
	if s.inner.BaseURL != identity.Endpoint || s.inner.Model != identity.APIModel {
		t.Fatalf("leased inference = %s/%s, want %s/%s", s.inner.BaseURL, s.inner.Model, identity.Endpoint, identity.APIModel)
	}
	if s.inner.Token != "secret" {
		t.Fatalf("endpoint token = %q, want secret", s.inner.Token)
	}
	if s.router.Fallback == nil || s.router.Fallback.BaseURL != "http://lan.example/v1" {
		t.Fatalf("auto fallback = %+v, want LAN safety fallback", s.router.Fallback)
	}
}
