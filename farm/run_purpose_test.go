package farm

import (
	"encoding/json"
	"testing"
)

func TestRunPolicyForCarriesPurposeIndependently(t *testing.T) {
	spec := Spec{
		RunID:         "debug-completionist-champion",
		Planner:       "llm",
		PlayStyle:     "completionist",
		Purpose:       "debug_coverage",
		Goal:          GoalFrom("elite-four"),
		RiskTolerance: "balanced",
	}
	policy := RunPolicyFor(spec)
	if policy.PlayStyle != "completionist" || policy.Purpose != "debug_coverage" || policy.Goal != "elite-four" {
		t.Fatalf("policy = %+v, want independent style/purpose/goal", policy)
	}
}

func TestSpecPurposeRoundTripsOnWire(t *testing.T) {
	want := Spec{RunID: "debug-run", Planner: "llm", Purpose: "debug_coverage"}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Spec
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Purpose != want.Purpose {
		t.Fatalf("purpose = %q, want %q; wire=%s", got.Purpose, want.Purpose, data)
	}
}
