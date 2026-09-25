package farm

import "testing"

func TestRunContextArtifactRoundTrip(t *testing.T) {
	const runID = "run-context-round-trip"

	spec := Spec{
		RunID:           runID,
		Seed:            42,
		Planner:         "llm",
		Starter:         "squirtle",
		Dest:            "champion",
		Goal:            GoalFrom("Beat the Elite Four and Champion."),
		PlayStyle:       "adventure",
		Purpose:         RunPurposeDebugCoverage,
		RiskTolerance:   "careful",
		WildEncounters:  "train",
		LLMProfile:      "qwen",
		ReasoningEffort: "high",
	}
	artifact, err := NewRunContextArtifact(spec)
	if err != nil {
		t.Fatalf("NewRunContextArtifact: %v", err)
	}
	if artifact.Name != RunContextArtifactName {
		t.Fatalf("artifact name = %q, want %q", artifact.Name, RunContextArtifactName)
	}

	got, ok, err := DecodeRunContext(FinishReport{Artifacts: []Artifact{artifact}})
	if err != nil {
		t.Fatalf("DecodeRunContext: %v", err)
	}
	if !ok {
		t.Fatal("DecodeRunContext did not find artifact")
	}
	if got.Planner != spec.Planner || got.Goal != spec.Goal.String() || got.LLMProfile != spec.LLMProfile || got.ReasoningEffort != spec.ReasoningEffort {
		t.Fatalf("run context core fields = %+v", got)
	}
	if got.PlayStyle != "adventure" || got.Purpose != RunPurposeDebugCoverage || got.RiskTolerance != "careful" || got.WildEncounters != "train" {
		t.Fatalf("run policy fields = %+v", got)
	}
	if got.Seed != spec.Seed || got.Starter != spec.Starter || got.Dest != spec.Dest {
		t.Fatalf("run identity fields = %+v", got)
	}
}

func TestDecodeRunContextLegacyReport(t *testing.T) {
	_, ok, err := DecodeRunContext(FinishReport{})
	if err != nil {
		t.Fatalf("DecodeRunContext legacy report: %v", err)
	}
	if ok {
		t.Fatal("legacy report unexpectedly has run context")
	}
}
