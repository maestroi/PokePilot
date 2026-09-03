package agent

import "strings"

// LLMProfile selects which endpoint family a leased run uses. Workers expose
// the default/LAN endpoint via POKEPILOT_LLM_* and an optional GPU endpoint
// via POKEPILOT_LLM_GPU_* or, for older deploys, POKEPILOT_LLM_FALLBACK_*.
type LLMProfile string

const (
	LLMProfileDefault LLMProfile = "default"
	LLMProfileGPU     LLMProfile = "gpu"
	LLMProfileAuto    LLMProfile = "auto"
)

// NormalizeLLMProfile maps queue/form values onto the three supported modes.
// Empty and unknown values mean default (primary env only).
func NormalizeLLMProfile(s string) LLMProfile {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(LLMProfileGPU):
		return LLMProfileGPU
	case string(LLMProfileAuto):
		return LLMProfileAuto
	default:
		return LLMProfileDefault
	}
}

// LLMProfileLabel renders a profile for operator surfaces.
func LLMProfileLabel(p LLMProfile) string {
	switch p {
	case LLMProfileGPU:
		return "GPU"
	case LLMProfileAuto:
		return "Auto (GPU → LAN)"
	default:
		return "Default (LAN)"
	}
}

// defaultLLMConfigFromEnv reads the primary POKEPILOT_LLM_* endpoint.
func defaultLLMConfigFromEnv() LLMConfig {
	p := NewLLMPlanner()
	return LLMConfig{
		BaseURL:   p.BaseURL,
		Model:     p.Model,
		Token:     p.Token,
		NoThink:   p.NoThink,
		MaxTokens: p.MaxTokens,
		Timeout:   p.Timeout,
	}
}

// gpuLLMConfigFromEnv reads an optional GPU endpoint. POKEPILOT_LLM_GPU_*
// wins; POKEPILOT_LLM_FALLBACK_* is the legacy slot farm deploys use.
func gpuLLMConfigFromEnv(defaults LLMConfig) (LLMConfig, bool) {
	if c, ok := OptionalLLMConfigFromEnv("POKEPILOT_LLM_GPU_", defaults); ok {
		return c, true
	}
	return OptionalLLMConfigFromEnv("POKEPILOT_LLM_FALLBACK_", defaults)
}

// ResolveLLMEndpoints maps a profile onto primary and optional fallback
// endpoint configs. Auto matches make run-llm-auto: GPU primary with the
// default/LAN endpoint as transport fallback.
func ResolveLLMEndpoints(profile LLMProfile) (primary LLMConfig, fallback *LLMConfig) {
	lan := defaultLLMConfigFromEnv()
	gpu, hasGPU := gpuLLMConfigFromEnv(lan)
	switch profile {
	case LLMProfileGPU:
		if hasGPU {
			return gpu, nil
		}
		return lan, nil
	case LLMProfileAuto:
		if hasGPU {
			fb := lan
			return gpu, &fb
		}
		return lan, nil
	default:
		return lan, nil
	}
}
