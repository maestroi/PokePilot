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

// NormalizeReasoningEffort maps a queue/form value onto a value
// LLMPlanner.ReasoningEffort accepts: "low"/"medium"/"high" (the
// reasoning_effort field), "off" (disables thinking outright via
// chat_template_kwargs, the same escape hatch NoThink gives the chooser),
// or "" (meaning: use the endpoint's configured default) for anything
// else, including "auto" and empty.
func NormalizeReasoningEffort(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low", "medium", "high", "off":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return ""
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
		BaseURL:         p.BaseURL,
		Model:           p.Model,
		Token:           p.Token,
		NoThink:         p.NoThink,
		MaxTokens:       p.MaxTokens,
		Timeout:         p.Timeout,
		ReasoningEffort: p.ReasoningEffort,
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
	return ResolveLLMEndpointsWithEffort(profile, "")
}

// ResolveLLMEndpointsWithEffort is ResolveLLMEndpoints plus a per-run
// reasoning_effort override (low/medium/high). Empty defers to the
// environment/"medium" default on both endpoints; a run-specified value
// wins over POKEPILOT_LLM_REASONING_EFFORT and POKEPILOT_LLM_GPU_*.
func ResolveLLMEndpointsWithEffort(profile LLMProfile, reasoningEffort string) (primary LLMConfig, fallback *LLMConfig) {
	lan := defaultLLMConfigFromEnv()
	gpu, hasGPU := gpuLLMConfigFromEnv(lan)
	if reasoningEffort != "" {
		lan.ReasoningEffort = reasoningEffort
		gpu.ReasoningEffort = reasoningEffort
	}
	switch profile {
	case LLMProfileGPU:
		if hasGPU {
			return gpu, nil
		}
		// GPU-only with no GPU endpoint configured must not silently become
		// the LAN model. An empty config fails the first ask instead.
		return LLMConfig{}, nil
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
