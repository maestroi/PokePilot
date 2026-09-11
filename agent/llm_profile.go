package agent

import (
	"os"
	"strings"
)

// LLMProfile selects which inference resource family a leased run may use.
// The wire values are intentionally kept stable because they are persisted in
// run specs: "auto" is the normal 7900 XTX path, "gpu" is the explicitly
// selected 4090, and "default" is CPU/LAN only. LiteLLM remains supported when
// POKEPILOT_LLM_GATEWAY_URL is explicitly configured, but is no longer needed
// for the normal farm path.
type LLMProfile string

const (
	LLMProfileDefault LLMProfile = "default"
	LLMProfileGPU     LLMProfile = "gpu"
	LLMProfileAuto    LLMProfile = "auto"
)

const (
	gatewayModelAuto = "pokepilot-auto"
	gatewayModelGPU  = "pokepilot-4090"
	gatewayModelLAN  = "pokepilot-lan"
)

// NormalizeLLMProfile maps queue/form values onto the three supported modes.
// Empty and unknown values mean auto, which is the farm's normal 7900 XTX
// route. The persisted names predate the current hardware policy, so callers
// should use LLMProfileLabel for human-readable names.
func NormalizeLLMProfile(s string) LLMProfile {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(LLMProfileGPU):
		return LLMProfileGPU
	case string(LLMProfileDefault):
		return LLMProfileDefault
	case string(LLMProfileAuto), "":
		return LLMProfileAuto
	default:
		return LLMProfileAuto
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

// LLMProfileLabel renders the current hardware policy while keeping the stable
// persisted profile values out of the operator-facing UI.
func LLMProfileLabel(p LLMProfile) string {
	switch p {
	case LLMProfileGPU:
		return "RTX 4090"
	case LLMProfileDefault:
		return "CPU only"
	default:
		return "7900 XTX (CPU fallback)"
	}
}

// defaultLLMConfigFromEnv reads the primary POKEPILOT_LLM_* endpoint. In the
// farm deployment this is the CPU/LAN model.
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

// gpuLLMConfigFromEnv reads the direct 7900 XTX endpoint. POKEPILOT_LLM_GPU_*
// wins; POKEPILOT_LLM_FALLBACK_* is the legacy slot older farm deploys use.
func gpuLLMConfigFromEnv(defaults LLMConfig) (LLMConfig, bool) {
	if c, ok := OptionalLLMConfigFromEnv("POKEPILOT_LLM_GPU_", defaults); ok {
		return c, true
	}
	return OptionalLLMConfigFromEnv("POKEPILOT_LLM_FALLBACK_", defaults)
}

// gpu4090LLMConfigFromEnv reads the explicitly selected direct RTX 4090
// endpoint. It deliberately has its own prefix so reserving/selecting the 4090
// never changes the normal 7900 XTX route.
func gpu4090LLMConfigFromEnv(defaults LLMConfig) (LLMConfig, bool) {
	return OptionalLLMConfigFromEnv("POKEPILOT_LLM_4090_", defaults)
}

// gatewayLLMConfigFromEnv maps a profile onto a logical model group on the
// optional inference gateway. Only URL enables gateway mode. Keeping this path
// means LiteLLM can be re-enabled for experiments without making it the farm's
// normal request path.
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
// endpoint configs. With the gateway disabled (the farm default), Auto goes
// directly to the 7900 XTX and uses CPU/LAN only after a transport failure or
// the primary request timeout. GPU is the explicitly selected 4090 and Default
// is CPU only. If an operator explicitly enables the gateway, the historical
// logical model groups remain available.
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

	gpu7900, has7900 := gpuLLMConfigFromEnv(lan)
	gpu4090, has4090 := gpu4090LLMConfigFromEnv(lan)
	if reasoningEffort != "" {
		gpu7900.ReasoningEffort = reasoningEffort
		gpu4090.ReasoningEffort = reasoningEffort
	}
	switch profile {
	case LLMProfileGPU:
		if has4090 {
			return gpu4090, nil
		}
		// An explicitly selected 4090 must never silently become another
		// resource family when that endpoint is not configured.
		return LLMConfig{}, nil
	case LLMProfileDefault:
		return lan, nil
	default: // Auto is the normal direct 7900 XTX route.
		if has7900 {
			fb := lan
			return gpu7900, &fb
		}
		// Older/local deployments without a GPU endpoint remain usable.
		return lan, nil
	}
}
