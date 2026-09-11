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
		{"", LLMProfileAuto},
		{"default", LLMProfileDefault},
		{"GPU", LLMProfileGPU},
		{"auto", LLMProfileAuto},
		{"weird", LLMProfileAuto},
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
	t.Setenv("POKEPILOT_LLM_GPU_URL", "http://7900.example/v1")
	t.Setenv("POKEPILOT_LLM_GPU_MODEL", "7900-model")
	t.Setenv("POKEPILOT_LLM_GPU_TOKEN", "7900-token")
	t.Setenv("POKEPILOT_LLM_4090_URL", "http://4090.example/v1")
	t.Setenv("POKEPILOT_LLM_4090_MODEL", "4090-model")
	t.Setenv("POKEPILOT_LLM_4090_TOKEN", "4090-token")

	primary, fb := ResolveLLMEndpoints(LLMProfileDefault)
	if primary.BaseURL != "http://lan.example/v1" || primary.Model != "lan-model" || fb != nil {
		t.Fatalf("default = primary %+v fallback %v", primary, fb)
	}

	primary, fb = ResolveLLMEndpoints(LLMProfileGPU)
	if primary.BaseURL != "http://4090.example/v1" || primary.Model != "4090-model" || fb != nil {
		t.Fatalf("gpu/4090 = primary %+v fallback %v", primary, fb)
	}

	primary, fb = ResolveLLMEndpoints(LLMProfileAuto)
	if primary.BaseURL != "http://7900.example/v1" || primary.Model != "7900-model" || fb == nil {
		t.Fatalf("auto/7900 primary = %+v fallback %v", primary, fb)
	}
	if fb.BaseURL != "http://lan.example/v1" || fb.Model != "lan-model" || fb.Token != "lan-token" {
		t.Fatalf("auto CPU fallback = %+v", fb)
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
		profile      LLMProfile
		model        string
		wantFallback bool
	}{
		{LLMProfileAuto, "pokepilot-auto", true},
		{LLMProfileGPU, "pokepilot-4090", false},
		{LLMProfileDefault, "pokepilot-lan", true},
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
		if tc.wantFallback {
			if fb == nil || fb.BaseURL != "http://direct-lan.example/v1" || fb.Model != "direct-model" {
				t.Fatalf("%s gateway fallback = %+v, want direct LAN safety fallback", tc.profile, fb)
			}
		} else if fb != nil {
			t.Fatalf("%s gateway fallback = %+v, want nil", tc.profile, fb)
		}
	}
}

func TestResolveLLMEndpointsGatewayRecoveryEffort(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "http://litellm:4000/v1")
	t.Setenv("POKEPILOT_LLM_GATEWAY_RECOVERY_REASONING_EFFORT", "off")
	primary, _ := ResolveLLMEndpoints(LLMProfileGPU)
	if primary.RecoveryReasoningEffort != "off" {
		t.Fatalf("gateway recovery effort = %q, want off (blackout must not silently escalate to medium)", primary.RecoveryReasoningEffort)
	}
}

func TestResolveLLMEndpointsGatewayModelOverrides(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "http://litellm:4000/v1")
	t.Setenv("POKEPILOT_LLM_GATEWAY_AUTO_MODEL", "custom-auto")
	t.Setenv("POKEPILOT_LLM_GATEWAY_GPU_MODEL", "custom-4090")
	t.Setenv("POKEPILOT_LLM_GATEWAY_LAN_MODEL", "custom-lan")

	for _, tc := range []struct {
		profile LLMProfile
		model   string
	}{
		{LLMProfileAuto, "custom-auto"},
		{LLMProfileGPU, "custom-4090"},
		{LLMProfileDefault, "custom-lan"},
	} {
		primary, _ := ResolveLLMEndpoints(tc.profile)
		if primary.Model != tc.model {
			t.Fatalf("%s gateway model = %q, want %q", tc.profile, primary.Model, tc.model)
		}
	}
}

func TestResolveLLMEndpointsPrefers7900GPUPrefix(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "")
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("POKEPILOT_LLM_GPU_URL", "http://explicit-7900/v1")
	t.Setenv("POKEPILOT_LLM_GPU_MODEL", "explicit-7900-model")
	t.Setenv("POKEPILOT_LLM_FALLBACK_URL", "http://legacy-gpu/v1")
	t.Setenv("POKEPILOT_LLM_FALLBACK_MODEL", "legacy-gpu-model")

	primary, _ := ResolveLLMEndpoints(LLMProfileAuto)
	if primary.BaseURL != "http://explicit-7900/v1" || primary.Model != "explicit-7900-model" {
		t.Fatalf("auto/7900 primary = %+v, want explicit GPU env", primary)
	}
}

func TestResolveLLMEndpoints4090IsExplicit(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "")
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("POKEPILOT_LLM_GPU_URL", "http://7900.example/v1")
	t.Setenv("POKEPILOT_LLM_4090_URL", "http://4090.example/v1")
	t.Setenv("POKEPILOT_LLM_4090_MODEL", "4090-model")

	primary, fb := ResolveLLMEndpoints(LLMProfileGPU)
	if primary.BaseURL != "http://4090.example/v1" || primary.Model != "4090-model" {
		t.Fatalf("gpu/4090 primary = %+v", primary)
	}
	if fb != nil {
		t.Fatalf("gpu/4090 fallback = %+v, want nil", fb)
	}
}

func TestResolveLLMEndpointsAutoWithout7900FallsBackToCPU(t *testing.T) {
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
		t.Fatalf("auto without 7900 = primary %+v fallback %v", primary, fb)
	}
}

func TestResolveLLMEndpoints4090WithoutEndpointDoesNotUseOtherResources(t *testing.T) {
	t.Setenv("POKEPILOT_LLM_GATEWAY_URL", "")
	for _, key := range []string{
		"POKEPILOT_LLM_4090_URL", "POKEPILOT_LLM_4090_MODEL",
	} {
		os.Unsetenv(key)
	}
	t.Setenv("POKEPILOT_LLM_URL", "http://lan.example/v1")
	t.Setenv("POKEPILOT_LLM_MODEL", "lan-model")
	t.Setenv("POKEPILOT_LLM_GPU_URL", "http://7900.example/v1")
	t.Setenv("llm_token", "")

	primary, fb := ResolveLLMEndpoints(LLMProfileGPU)
	if primary.BaseURL != "" || primary.Model != "" {
		t.Fatalf("4090 without endpoint = %+v, must not silently use 7900 or CPU", primary)
	}
	if fb != nil {
		t.Fatalf("4090 fallback = %+v, want nil", fb)
	}
}
