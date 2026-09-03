package agent

import (
	"os"
	"testing"
)

func TestNormalizeLLMProfile(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want LLMProfile
	}{
		{"", LLMProfileDefault},
		{"default", LLMProfileDefault},
		{"GPU", LLMProfileGPU},
		{"auto", LLMProfileAuto},
		{"weird", LLMProfileDefault},
	} {
		if got := NormalizeLLMProfile(tc.in); got != tc.want {
			t.Fatalf("NormalizeLLMProfile(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveLLMEndpointsProfiles(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("llm_token", "lan-token")
	t.Setenv("POKEPILOT_LLM_FALLBACK_URL", "http://gpu.example/v1")
	t.Setenv("POKEPILOT_LLM_FALLBACK_MODEL", "gpu-model")
	t.Setenv("POKEPILOT_LLM_FALLBACK_TOKEN", "gpu-token")

	primary, fb := ResolveLLMEndpoints(LLMProfileDefault)
	if primary.BaseURL != "http://lan.example/v1" || primary.Model != "lan-model" || fb != nil {
		t.Fatalf("default = primary %+v fallback %v", primary, fb)
	}

	primary, fb = ResolveLLMEndpoints(LLMProfileGPU)
	if primary.BaseURL != "http://gpu.example/v1" || primary.Model != "gpu-model" || fb != nil {
		t.Fatalf("gpu = primary %+v fallback %v", primary, fb)
	}

	primary, fb = ResolveLLMEndpoints(LLMProfileAuto)
	if primary.BaseURL != "http://gpu.example/v1" || primary.Model != "gpu-model" || fb == nil {
		t.Fatalf("auto primary = %+v fallback %v", primary, fb)
	}
	if fb.BaseURL != "http://lan.example/v1" || fb.Model != "lan-model" || fb.Token != "lan-token" {
		t.Fatalf("auto fallback = %+v", fb)
	}
}

func TestResolveLLMEndpointsPrefersGPUPrefix(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("POKEPILOT_LLM_GPU_URL", "http://explicit-gpu/v1")
	t.Setenv("POKEPILOT_LLM_GPU_MODEL", "explicit-gpu-model")
	t.Setenv("POKEPILOT_LLM_FALLBACK_URL", "http://legacy-gpu/v1")
	t.Setenv("POKEPILOT_LLM_FALLBACK_MODEL", "legacy-gpu-model")

	primary, _ := ResolveLLMEndpoints(LLMProfileGPU)
	if primary.BaseURL != "http://explicit-gpu/v1" || primary.Model != "explicit-gpu-model" {
		t.Fatalf("gpu primary = %+v, want explicit GPU env", primary)
	}
}

func TestResolveLLMEndpointsAutoWithoutGPUFallsBackToDefault(t *testing.T) {
	for _, key := range []string{
		"POKEPILOT_LLM_GPU_URL", "POKEPILOT_LLM_GPU_MODEL",
		"POKEPILOT_LLM_FALLBACK_URL", "POKEPILOT_LLM_FALLBACK_MODEL",
	} {
		os.Unsetenv(key)
	}
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")

	primary, fb := ResolveLLMEndpoints(LLMProfileAuto)
	if primary.BaseURL != "http://lan.example/v1" || fb != nil {
		t.Fatalf("auto without gpu = primary %+v fallback %v", primary, fb)
	}
}
