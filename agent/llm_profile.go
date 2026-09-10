package agent

import (
	"os"
	"strings"
)

// LLMProfile selects which inference resource family a leased run may use.
// The profile expresses operator intent; when POKEPILOT_LLM_GATEWAY_URL is set
// the gateway owns the physical endpoints and failover order. Without a
// gateway, the historical direct LAN/GPU environment remains supported.
type LLMProfile string

const (
	LLMProfileDefault LLMProfile = "default"
	LLMProfileGPU     LLMProfile = "gpu"
	LLMProfileAuto    LLMProfile = "auto"
)

const (
	gatewayModelAuto = "pokepilot-auto"
	gatewayModelGPU  = "pokepilot-7900xtx"
	gatewayModelLAN  = "pokepilot-lan"
)

// NormalizeLLMProfile maps queue/form values onto the three supported modes.
// Empty and unknown values mean default (LAN-only / primary env only).
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

// LLMProfileLabel renders the resource intent without coupling generic runtime
// code to a particular graphics-card model.
func LLMProfileLabel(p LLMProfile) string {
	switch p {
	case LLMProfileGPU:
		return "Dedicated GPU only"
	case LLMProfileAuto:
		return "Auto (GPU pool → LAN)"
	default:
		return "LAN only"
	}
}

// defaultLLMConfigFromEnv reads the primary POKEPILOT_LLM_* endpoint.
func defaultLLMConfigFromEnv() LLMConfig {
	p := NewLLMPlanner()
	return LLMConfig{
		BaseURL:                 p.BaseURL,
		Model:                   p.Model,
		Token:                   p.Token,
		NoThink:                 p.NoThink,
		MaxTokens:               p.MaxTokens,
		Timeout:                 p.Timeout,
		ReasoningEffort:         p.ReasoningEffort,
		RecoveryReasoningEffort: p.RecoveryReasoningEffort,
	}
}

// gpuLLMConfigFromEnv reads an optional direct GPU endpoint. POKEPILOT_LLM_GPU_*
// wins; POKEPILOT_LLM_FALLBACK_* is the legacy slot farm deploys use.
func gpuLLMConfigFromEnv(defaults LLMConfig) (LLMConfig, bool) {
	if c, ok := OptionalLLMConfigFromEnv("POKEPILOT_LLM_GPU_", defaults); ok {
		return c, true
	}
	return OptionalLLMConfigFromEnv("POKEPILOT_LLM_FALLBACK_", defaults)
}

// gatewayLLMConfigFromEnv maps a profile onto a logical model group on the
// inference gateway. Only URL enables gateway mode. The common gateway config
// controls transport settings; per-profile MODEL vars can rename aliases
// without teaching PokePilot about physical endpoints.
func gatewayLLMConfigFromEnv(profile LLMProfile, defaults LLMConfig) (LLMConfig, bool) {
	c, ok := OptionalLLMConfigFromEnv("POKEPILOT_LLM_GATEWAY_", defaults)
	if !ok {
		return LLMConfig{}, false
	}

	model := gatewayModelLAN
	modelEnv := "POKEPILOT_LLM_GATEWAY_LAN_MODEL"
	switch profile {
	case LLMProfileGPU:
		model = gatewayModelGPU
		modelEnv = "POKEPILOT_LLM_GATEWAY_GPU_MODEL"
	case LLMProfileAuto:
		model = gatewayModelAuto
		modelEnv = "POKEPILOT_LLM_GATEWAY_AUTO_MODEL"
	}
	if override := strings.TrimSpace(os.Getenv(modelEnv)); override != "" {
		model = override
	}
	c.Model = model
	return c, true
}

// ResolveLLMEndpoints maps a profile onto primary and optional fallback
// endpoint configs. In gateway mode the logical model group owns resource
// failover. LAN-capable profiles retain the direct LAN endpoint as a final
// transport fallback in case the gateway service itself is unavailable.
// Without a gateway, Auto preserves the historical direct behavior: GPU
// primary with default/LAN transport fallback.
func ResolveLLMEndpoints(profile LLMProfile) (primary LLMConfig, fallback *LLMConfig) {
	return ResolveLLMEndpointsWithEffort(profile, "")
}

// ResolveLLMEndpointsWithEffort is ResolveLLMEndpoints plus a per-run
// reasoning_effort override. A run-specified value wins over endpoint defaults
// in either direct or gateway mode.
func ResolveLLMEndpointsWithEffort(profile LLMProfile, reasoningEffort string) (primary LLMConfig, fallback *LLMConfig) {
	lan := defaultLLMConfigFromEnv()
	if reasoningEffort != "" {
		lan.ReasoningEffort = reasoningEffort
	}
	if gateway, ok := gatewayLLMConfigFromEnv(profile, lan); ok {
		if reasoningEffort != "" {
			gateway.ReasoningEffort = reasoningEffort
		}
		if profile == LLMProfileGPU {
			return gateway, nil
		}
		fb := lan
		return gateway, &fb
	}

	gpu, hasGPU := gpuLLMConfigFromEnv(lan)
	if reasoningEffort != "" {
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
