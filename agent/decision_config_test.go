package agent

import (
	"errors"
	"testing"
)

func TestDecisionSettingsDefaultKeepsExistingPlanner(t *testing.T) {
	t.Setenv("POKEPILOT_DECISION_BACKEND", "")
	settings := DecisionSettingsFromEnv()
	if settings.Engine != nil || settings.ObjectiveSelection || settings.FailureRecovery {
		t.Fatalf("default settings = %#v, want typed backend disabled", settings)
	}
}

func TestDecisionSettingsKeepsBackendAndFeatureFlagsIndependent(t *testing.T) {
	t.Setenv("POKEPILOT_DECISION_BACKEND", "system-one")
	t.Setenv("POKEPILOT_DECISION_URL", "http://decision.example/v1")
	t.Setenv("POKEPILOT_DECISION_MODEL", "tiny-system-one")
	t.Setenv("POKEPILOT_DECISION_OBJECTIVES", "1")
	t.Setenv("POKEPILOT_DECISION_FAILURES", "0")
	t.Setenv("POKEPILOT_DECISION_MIN_CONFIDENCE", "0.72")

	settings := DecisionSettingsFromEnv()
	engine, ok := settings.Engine.(*OpenAIDecisionEngine)
	if !ok {
		t.Fatalf("engine = %T, want OpenAIDecisionEngine", settings.Engine)
	}
	if engine.BaseURL != "http://decision.example/v1" || engine.Model != "tiny-system-one" {
		t.Fatalf("engine config = url %q model %q", engine.BaseURL, engine.Model)
	}
	if !settings.ObjectiveSelection || settings.FailureRecovery {
		t.Fatalf("feature flags = objectives %v failures %v", settings.ObjectiveSelection, settings.FailureRecovery)
	}
	if settings.MinConfidence != 0.72 {
		t.Fatalf("min confidence = %v, want 0.72", settings.MinConfidence)
	}
}

func TestDecisionSettingsSelectsJevWithoutPersistingCredential(t *testing.T) {
	t.Setenv("POKEPILOT_DECISION_BACKEND", "jev")
	t.Setenv("POKEPILOT_DECISION_URL", "https://decision.example/v1")
	t.Setenv("POKEPILOT_DECISION_MODEL", "jev-preview")
	t.Setenv("POKEPILOT_DECISION_TOKEN", "")
	t.Setenv("TYPESAFE_API_KEY", "secret-from-env")
	t.Setenv("POKEPILOT_DECISION_TIMEOUT", "750ms")

	settings := DecisionSettingsFromEnv()
	engine, ok := settings.Engine.(*JevDecisionEngine)
	if !ok {
		t.Fatalf("engine = %T, want JevDecisionEngine", settings.Engine)
	}
	if settings.Backend != "jev" || engine.BaseURL != "https://decision.example/v1" || engine.Model != "jev-preview" {
		t.Fatalf("settings = %#v engine = %#v", settings, engine)
	}
	if engine.Token != "secret-from-env" {
		t.Fatalf("token source was not TYPESAFE_API_KEY")
	}
	if engine.Timeout.String() != "750ms" {
		t.Fatalf("timeout = %s, want 750ms", engine.Timeout)
	}
}

func TestDecisionSettingsForRunSelection(t *testing.T) {
	t.Setenv("POKEPILOT_DECISION_BACKEND", "jev")
	t.Setenv("POKEPILOT_DECISION_MIN_CONFIDENCE", "")
	t.Setenv("POKEPILOT_DECISION_TOKEN", "")
	t.Setenv("TYPESAFE_API_KEY", "")

	// No run selection keeps the runner's environment default.
	settings, err := DecisionSettingsFor(DecisionSelection{})
	if err != nil || settings.Backend != "jev" {
		t.Fatalf("unselected = %+v, %v; want env default", settings, err)
	}

	// An explicit off wins over the environment.
	settings, err = DecisionSettingsFor(DecisionSelection{Backend: "off"})
	if err != nil || settings.Engine != nil || settings.ObjectiveSelection || settings.FailureRecovery {
		t.Fatalf("off = %+v, %v", settings, err)
	}

	// Selecting Jev on a runner without the key fails instead of silently
	// running a different experiment.
	if _, err := DecisionSettingsFor(DecisionSelection{Backend: "jev", FailureRecovery: true}); !errors.Is(err, ErrDecisionCredentialsMissing) {
		t.Fatalf("jev without key err = %v", err)
	}

	t.Setenv("TYPESAFE_API_KEY", "test-key")
	settings, err = DecisionSettingsFor(DecisionSelection{Backend: "jev", ObjectiveSelection: true})
	if err != nil {
		t.Fatal(err)
	}
	jev, ok := settings.Engine.(*JevDecisionEngine)
	if !ok || jev.Token != "test-key" || settings.Backend != "jev" {
		t.Fatalf("jev engine = %#v backend=%q", settings.Engine, settings.Backend)
	}
	if !settings.ObjectiveSelection || settings.FailureRecovery || settings.MinConfidence != defaultDecisionMinConfidence {
		t.Fatalf("run flags not applied: %+v", settings)
	}
	settings, err = DecisionSettingsFor(DecisionSelection{Backend: "jev", MinConfidence: 0.8})
	if err != nil || settings.MinConfidence != 0.8 {
		t.Fatalf("run confidence = %+v, %v", settings, err)
	}

	if _, err := DecisionSettingsFor(DecisionSelection{Backend: "gpt"}); !errors.Is(err, ErrDecisionDisabled) {
		t.Fatalf("unknown backend err = %v", err)
	}
}
