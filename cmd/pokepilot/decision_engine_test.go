package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

func TestDecisionSelectionForRunSpec(t *testing.T) {
	if got := decisionSelectionFor(nil); got != (agent.DecisionSelection{}) {
		t.Fatalf("nil selection = %+v, want runner default", got)
	}
	got := decisionSelectionFor(&farm.DecisionEngineSpec{Backend: "TypeSafe", Objectives: true, Failures: true, MinConfidence: 0.7})
	want := agent.DecisionSelection{Backend: farm.DecisionBackendJev, Mode: farm.DecisionModeActive, ObjectiveSelection: true, FailureRecovery: true, MinConfidence: 0.7}
	if got != want {
		t.Fatalf("selection = %+v, want %+v", got, want)
	}
	got = decisionSelectionFor(&farm.DecisionEngineSpec{Backend: "jev", Mode: "shadow", Battles: true})
	if got.Mode != farm.DecisionModeShadow || !got.Battles {
		t.Fatalf("shadow selection = %+v", got)
	}
	got = decisionSelectionFor(&farm.DecisionEngineSpec{Backend: "jev", Deployment: "typesafe-jev", Inference: &farm.InferenceIdentity{
		Endpoint: "https://api.typesafe.ai/v1", APIModel: "jev-2", TokenEnv: "JEV_KEY",
	}})
	if got.Endpoint != "https://api.typesafe.ai/v1" || got.Model != "jev-2" || got.TokenEnv != "JEV_KEY" {
		t.Fatalf("deployment selection = %+v", got)
	}
	// A backend this runner does not know must fail resolution, never fall
	// back to the environment default.
	t.Setenv("POKEPILOT_DECISION_BACKEND", "")
	if _, err := agent.DecisionSettingsFor(decisionSelectionFor(&farm.DecisionEngineSpec{Backend: "future-engine"})); !errors.Is(err, agent.ErrDecisionDisabled) {
		t.Fatalf("unknown backend err = %v", err)
	}
	if _, err := agent.DecisionSettingsFor(decisionSelectionFor(&farm.DecisionEngineSpec{Backend: "jev", Mode: "future-mode"})); !errors.Is(err, agent.ErrDecisionDisabled) {
		t.Fatalf("unknown mode err = %v", err)
	}
}

func TestFarmRunAppliesRunDecisionSelection(t *testing.T) {
	src, err := os.ReadFile("farm.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, want := range []string{
		"agent.DecisionSettingsFor(decisionSelectionFor(spec.DecisionEngine))",
		"stats.decision = decision",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("runFarmLLM no longer contains %q", want)
		}
	}
}
