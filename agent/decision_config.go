package agent

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// DecisionSettings keeps typed decisions orthogonal to the main strategic
// model. Existing generative behavior remains the default until a backend is
// explicitly selected.
type DecisionSettings struct {
	Engine             DecisionEngine
	Backend            string
	ObjectiveSelection bool
	FailureRecovery    bool
	MinConfidence      float64
}

func DecisionSettingsFromEnv() DecisionSettings {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_BACKEND")))
	settings := DecisionSettings{
		Backend:       backend,
		MinConfidence: 0.65,
	}
	if value := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_MIN_CONFIDENCE")); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed >= 0 && parsed <= 1 {
			settings.MinConfidence = parsed
		}
	}

	switch backend {
	case "", "off", "disabled", "generative":
		return settings
	case "system-one", "system_one", "local", "openai", "openai-compatible":
		engine := NewOpenAIDecisionEngineFromEnv()
		if value := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_TIMEOUT")); value != "" {
			if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
				engine.Timeout = parsed
			}
		}
		if value := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_MAX_TOKENS")); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
				engine.MaxTokens = parsed
			}
		}
		settings.Engine = engine
		settings.Backend = engine.Backend
		settings.FailureRecovery = decisionEnvBool("POKEPILOT_DECISION_FAILURES", true)
		settings.ObjectiveSelection = decisionEnvBool("POKEPILOT_DECISION_OBJECTIVES", false)
		return settings
	case "jev", "typesafe", "typesafe-jev", "system-one-jev", "system_one_jev":
		engine := NewJevDecisionEngineFromEnv()
		if value := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_TIMEOUT")); value != "" {
			if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
				engine.Timeout = parsed
			}
		}
		settings.Engine = engine
		settings.Backend = engine.Backend
		settings.FailureRecovery = decisionEnvBool("POKEPILOT_DECISION_FAILURES", true)
		settings.ObjectiveSelection = decisionEnvBool("POKEPILOT_DECISION_OBJECTIVES", false)
		return settings
	default:
		// Unknown values deliberately leave the experimental backend disabled.
		// They cannot silently replace the stable generative planner.
		return settings
	}
}

func decisionEnvBool(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	switch value {
	case "0", "false", "no", "off", "disabled":
		return false
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return fallback
	}
}
