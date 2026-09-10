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
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "")
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

func TestResolveLLMEndpointsGatewayProfiles(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_URL", "http://direct-lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "direct-model")
	t.Setenv("llm_token", "direct-token")
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "http://litellm:4000/v1")
	t.Setenv("POKEPILOT_LLM_GATEWAY_TOKEN", "gateway-token")
	t.Setenv("POKEPILOT_LLM_GATEWAY_TIMEOUT", "45s")

	for _, tc := range []struct {
		profile LLMProfile
		model   string
	}{
		{LLMProfileAuto, "pokepilot-auto"},
		{LLMProfileGPU, "pokepilot-7900xtx"},
		{LLMProfileDefault, "pokepilot-lan"},
	} {
		primary, fb := ResolveLLMEndpoints(tc.profile)
		if primary.BaseURL != "http://litellm:4000/v1" || primary.Model != tc.model {
			t.Fatalf("%s gateway = %+v, want model %q", tc.profile, primary, tc.model)
		}
		if primary.Token != "gateway-token" {
			t.Fatalf("%s gateway token = %q", tc.profile, primary.Token)
		}
		if primary.Timeout.String() != "45s" {
			t.Fatalf("%s gateway timeout = %s", tc.profile, primary.Timeout)
		}
		if fb != nil {
			t.Fatalf("%s gateway fallback = %+v, want gateway-owned failover", tc.profile, fb)
		}
	}
}

func TestResolveLLMEndpointsGatewayModelOverrides(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "http://litellm:4000/v1")
	t.Setenv("POKEPILOT_LLM_GATEWAY_AUTO_MODEL", "custom-auto")
	t.Setenv("POKEPILOT_LLM_GATEWAY_GPU_MODEL", "custom-dedicated")
	t.Setenv("POKEPILOT_LLM_GATEWAY_LAN_MODEL", "custom-lan")

	for _, tc := range []struct {
		profile LLMProfile
		model   string
	}{
		{LLMProfileAuto, "custom-auto"},
		{LLMProfileGPU, "custom-dedicated"},
		{LLMProfileDefault, "custom-lan"},
	} {
		primary, _ := ResolveLLMEndpoints(tc.profile)
		if primary.Model != tc.model {
			t.Fatalf("%s gateway model = %q, want %q", tc.profile, primary.Model, tc.model)
		}
	}
}

func TestResolveLLMEndpointsPrefersGPUPrefix(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "")
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
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "")
	for _, key := range []string{
		"POKEPILOT_LLM_GPU_URL", "POKEPILOT_LLM_GPU_MODEL",
		"POKEPILOT_LLM_FALLBACK_URL", "POKEPILOT_LLM_FALLBACK_MODEL",
	} {
		os.Unsetenv(key)
	}
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("llm_token", "")

	primary, fb := ResolveLLMEndpoints(LLMProfileAuto)
	if primary.BaseURL != "http://lan.example/v1" || fb != nil {
		t.Fatalf("auto without gpu = primary %+v fallback %v", primary, fb)
	}
}

func TestResolveLLMEndpointsGPUWithoutGPUDoesNotUseLAN(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "")
	for _, key := range []string{
		"POKEPILOT_LLM_GPU_URL", "POKEPILOT_LLM_GPU_MODEL",
		"POKEPILOT_LLM_FALLBACK_URL", "POKEPILOT_LLM_FALLBACK_MODEL",
	} {
		os.Unsetenv(key)
	}
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("llm_token", "")

	primary, fb := ResolveLLMEndpoints(LLMProfileGPU)
	if primary.BaseURL == "http://lan.example/v1" || primary.Model == "lan-model" {
		t.Fatalf("gpu without gpu endpoint = %+v, must not silently use LAN", primary)
	}
	if fb != nil {
		t.Fatalf("gpu fallback = %+v, want nil", fb)
	}
}
