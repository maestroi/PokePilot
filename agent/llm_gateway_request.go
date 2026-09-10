package agent

import (
	"encoding/json"
	"os"
	"strings"
)

// MarshalJSON adds LiteLLM's request-scoped allowlist only for strategist
// requests routed through one of PokePilot's logical gateway model aliases.
//
// LiteLLM validates OpenAI-shaped parameters before forwarding requests to a
// custom OpenAI-compatible backend. In current stable releases,
// allowed_openai_params configured on a model deployment is not reliably
// consulted for reasoning_effort, while the same allowlist in the request is.
// The physical Qwen servers do support reasoning_effort, so dropping it at the
// gateway would change planner behaviour and can put the strategist back onto
// the server's slow/default reasoning path.
//
// Direct endpoint requests must not receive this LiteLLM-only control field.
// The model alias is therefore the boundary: gatewayLLMConfigFromEnv selects
// one of these aliases (or its configured override), while direct LAN/GPU
// configs use the physical model id.
func (r chatRequest) MarshalJSON() ([]byte, error) {
	type chatRequestWire chatRequest

	if r.ReasoningEffort == "" || !isGatewayModelAlias(r.Model) {
		return json.Marshal(chatRequestWire(r))
	}

	return json.Marshal(struct {
		chatRequestWire
		AllowedOpenAIParams []string `json:"allowed_openai_params,omitempty"`
	}{
		chatRequestWire:     chatRequestWire(r),
		AllowedOpenAIParams: []string{"reasoning_effort"},
	})
}

func isGatewayModelAlias(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}

	for _, candidate := range []struct {
		envName  string
		fallback string
	}{
		{"POKEPILOT_LLM_GATEWAY_AUTO_MODEL", gatewayModelAuto},
		{"POKEPILOT_LLM_GATEWAY_GPU_MODEL", gatewayModelGPU},
		{"POKEPILOT_LLM_GATEWAY_LAN_MODEL", gatewayModelLAN},
	} {
		alias := strings.TrimSpace(os.Getenv(candidate.envName))
		if alias == "" {
			alias = candidate.fallback
		}
		if model == alias {
			return true
		}
	}
	return false
}
