package agent

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// LLMConfig is the endpoint-specific part of an LLMPlanner. Run context such
// as Goal, ExtraSystem and logs is deliberately not configuration: a router
// copies that live context from the primary planner immediately before an ask.
type LLMConfig struct {
	BaseURL   string
	Model     string
	Token     string
	NoThink   bool
	MaxTokens int
	Timeout   time.Duration
	// ReasoningEffort and RecoveryReasoningEffort carry the matching
	// LLMPlanner fields through the config path. Farm runs build their
	// planner via NewLLMPlannerFromConfig, never NewLLMPlanner, so the
	// same defaults must be applied here too or the strategist silently
	// reverts to "off" with no recovery escalation at all.
	ReasoningEffort         string
	RecoveryReasoningEffort string
}

// NewLLMPlannerFromConfig constructs a planner without reading process
// environment. Callers that need environment overrides can build the config
// first with OptionalLLMConfigFromEnv.
func NewLLMPlannerFromConfig(c LLMConfig) *LLMPlanner {
	effort := c.ReasoningEffort
	if effort == "" {
		effort = "off"
	}
	recovery := c.RecoveryReasoningEffort
	if recovery == "" {
		recovery = "medium"
	}
	return &LLMPlanner{
		BaseURL:                 c.BaseURL,
		Model:                   c.Model,
		Token:                   c.Token,
		NoThink:                 c.NoThink,
		MaxTokens:               c.MaxTokens,
		Timeout:                 c.Timeout,
		ReasoningEffort:         effort,
		RecoveryReasoningEffort: recovery,
	}
}

// OptionalLLMConfigFromEnv applies endpoint overrides under prefix. URL is
// the opt-in switch: an empty or whitespace-only URL means no endpoint is
// configured. Other absent values retain defaults. TOKEN intentionally uses
// LookupEnv so an explicitly empty token clears an inherited token.
func OptionalLLMConfigFromEnv(prefix string, defaults LLMConfig) (LLMConfig, bool) {
	baseURL := strings.TrimSpace(os.Getenv(prefix + "URL"))
	if baseURL == "" {
		return LLMConfig{}, false
	}

	c := defaults
	c.BaseURL = baseURL
	if model := strings.TrimSpace(os.Getenv(prefix + "MODEL")); model != "" {
		c.Model = model
	}
	if token, ok := os.LookupEnv(prefix + "TOKEN"); ok {
		c.Token = token
	}
	if v, ok := os.LookupEnv(prefix + "NO_THINK"); ok {
		c.NoThink = v != "" && v != "0"
	}
	if v := strings.TrimSpace(os.Getenv(prefix + "MAX_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.MaxTokens = n
		}
	}
	if v := strings.TrimSpace(os.Getenv(prefix + "TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Timeout = d
		}
	}
	if v := strings.TrimSpace(os.Getenv(prefix + "REASONING_EFFORT")); v != "" {
		c.ReasoningEffort = v
	}
	if v := strings.TrimSpace(os.Getenv(prefix + "RECOVERY_REASONING_EFFORT")); v != "" {
		c.RecoveryReasoningEffort = v
	}
	return c, true
}
