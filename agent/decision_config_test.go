package agent

import "testing"

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
