package agent

import (
	"errors"
	"fmt"
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
	// Battles asks the engine at battle turns; battle answers are only ever
	// observed, never executed.
	Battles       bool
	MinConfidence float64
	// Shadow consults the engine at every enabled decision point and records
	// its answer and agreement, but the existing policy keeps deciding.
	Shadow bool
}

// Mode names the settings' decision mode for run identity and telemetry.
func (s DecisionSettings) Mode() string {
	switch {
	case s.Engine == nil:
		return "off"
	case s.Shadow:
		return "shadow"
	default:
		return "active"
	}
}

// defaultDecisionMinConfidence is the confidence floor when neither the run
// nor POKEPILOT_DECISION_MIN_CONFIDENCE sets one.
const defaultDecisionMinConfidence = 0.65

// ErrDecisionCredentialsMissing reports that a run selected a typed backend
// whose credential is not configured on this runner. Failing the run is
// deliberate: silently running without the selected backend would make the
// run's recorded experiment identity a lie.
var ErrDecisionCredentialsMissing = errors.New("agent: typed decision backend credentials are not configured on this runner")

// DecisionSelection is a run's own typed-decision choice. Endpoints and
// credentials are never part of it; they come from the runner environment.
type DecisionSelection struct {
	Backend string
	// Mode is off, shadow or active; empty means active.
	Mode               string
	ObjectiveSelection bool
	FailureRecovery    bool
	Battles            bool
	MinConfidence      float64
}

func DecisionSettingsFromEnv() DecisionSettings {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_BACKEND")))
	settings := DecisionSettings{
		Backend:       backend,
		MinConfidence: envDecisionMinConfidence(),
	}
	engine, name := newDecisionEngineFromEnv(backend)
	if engine == nil {
		// Unknown values deliberately leave the experimental backend disabled.
		// They cannot silently replace the stable generative planner.
		return settings
	}
	settings.Engine = engine
	settings.Backend = name
	settings.FailureRecovery = decisionEnvBool("POKEPILOT_DECISION_FAILURES", true)
	settings.ObjectiveSelection = decisionEnvBool("POKEPILOT_DECISION_OBJECTIVES", false)
	switch strings.ToLower(strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_MODE"))) {
	case "shadow":
		settings.Shadow = true
		settings.Battles = decisionEnvBool("POKEPILOT_DECISION_BATTLES", false)
	case "off":
		return DecisionSettings{Backend: "off", MinConfidence: settings.MinConfidence}
	}
	return settings
}

// DecisionSettingsFor resolves a run's selection against this runner's
// environment. An empty backend keeps DecisionSettingsFromEnv, so runs that
// never chose a backend behave exactly as before; "off" disables typed
// decisions even when the runner environment enables them.
func DecisionSettingsFor(sel DecisionSelection) (DecisionSettings, error) {
	backend := strings.ToLower(strings.TrimSpace(sel.Backend))
	if backend == "" {
		return DecisionSettingsFromEnv(), nil
	}
	minConfidence := sel.MinConfidence
	if minConfidence <= 0 {
		minConfidence = envDecisionMinConfidence()
	}
	settings := DecisionSettings{Backend: backend, MinConfidence: minConfidence}
	var shadow bool
	switch strings.ToLower(strings.TrimSpace(sel.Mode)) {
	case "", "active":
	case "shadow":
		shadow = true
	case "off":
		backend = "off"
		settings.Backend = backend
	default:
		return settings, fmt.Errorf("%w: unknown mode %q", ErrDecisionDisabled, sel.Mode)
	}
	if backend == "off" {
		return settings, nil
	}
	if sel.Battles && !shadow {
		return settings, fmt.Errorf("%w: battle decisions support only shadow mode", ErrDecisionDisabled)
	}
	engine, name := newDecisionEngineFromEnv(backend)
	if engine == nil {
		return settings, fmt.Errorf("%w: unknown backend %q", ErrDecisionDisabled, sel.Backend)
	}
	if jev, ok := engine.(*JevDecisionEngine); ok && jev.Token == "" {
		return settings, fmt.Errorf("%w: backend %q needs TYPESAFE_API_KEY (or POKEPILOT_DECISION_TOKEN)", ErrDecisionCredentialsMissing, name)
	}
	settings.Engine = engine
	settings.Backend = name
	settings.ObjectiveSelection = sel.ObjectiveSelection
	settings.FailureRecovery = sel.FailureRecovery
	settings.Battles = sel.Battles
	settings.Shadow = shadow
	return settings, nil
}

// newDecisionEngineFromEnv builds the named backend from runner environment
// endpoints/credentials, or nil for off/unknown values.
func newDecisionEngineFromEnv(backend string) (DecisionEngine, string) {
	switch backend {
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
		return engine, engine.Backend
	case "jev", "typesafe", "typesafe-jev", "system-one-jev", "system_one_jev":
		engine := NewJevDecisionEngineFromEnv()
		if value := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_TIMEOUT")); value != "" {
			if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
				engine.Timeout = parsed
			}
		}
		return engine, engine.Backend
	}
	return nil, ""
}

func envDecisionMinConfidence() float64 {
	if value := strings.TrimSpace(os.Getenv("POKEPILOT_DECISION_MIN_CONFIDENCE")); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed >= 0 && parsed <= 1 {
			return parsed
		}
	}
	return defaultDecisionMinConfidence
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
