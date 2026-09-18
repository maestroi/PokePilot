package main

import (
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

func TestNewStatsPlannerHonorsLLMProfile(t *testing.T) {
	var legacy farm.Spec
	if err := json.Unmarshal([]byte(`{"run_id":"legacy"}`), &legacy); err != nil {
		t.Fatal(err)
	}
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


func TestNewStatsPlannerUsesLeasedInferenceIdentity(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GPU_URL", "http://legacy-7900.example/v1")
	t.Setenv("POKEPILOT_LLM_GPU_MODEL", "old-27b")
	t.Setenv("DISCOVERED_MODEL_TOKEN", "secret")

	var spec farm.Spec
	if err := json.Unmarshal([]byte(`{
		"run_id":"leased",
		"llm_profile":"auto",
		"inference":{
			"deployment_id":"primary",
			"model_id":"qwen3.5-9b",
			"compute":"RX 7900 XTX",
			"endpoint":"http://dynamic-gpu.example/v1",
			"api_model":"qwen3.5-9b",
			"token_env":"DISCOVERED_MODEL_TOKEN"
		}
	}`), &spec); err != nil {
		t.Fatal(err)
	}

	s := newStatsPlanner("auto", "", "Earn the Boulder Badge.", nil, nil, nil)
	if s.inner.BaseURL != "http://dynamic-gpu.example/v1" || s.inner.Model != "qwen3.5-9b" {
		t.Fatalf("leased primary = %s/%s", s.inner.BaseURL, s.inner.Model)
	}
	if s.inner.Token != "secret" {
		t.Fatalf("leased token = %q", s.inner.Token)
	}
	if s.router.Fallback != nil {
		t.Fatalf("first-class deployment must not silently fail over to a different inference identity: %+v", s.router.Fallback)
	}

	var reset farm.Spec
	if err := json.Unmarshal([]byte(`{"run_id":"reset"}`), &reset); err != nil {
		t.Fatal(err)
	}
}
