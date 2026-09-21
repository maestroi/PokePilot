package agent

import "testing"

func TestSemanticProgressAndCapabilityGoals(t *testing.T) {
	progress, err := ParseGoal("progress:silph_co_cleared")
	if err != nil || progress.Kind != GoalProgress {
		t.Fatalf("progress goal = %+v, %v", progress, err)
	}
	capability, err := ParseGoal("capability:surf")
	if err != nil || capability.Kind != GoalCapability {
		t.Fatalf("capability goal = %+v, %v", capability, err)
	}
	obs := Observation{
		Story: ProgressState{{ID: "silph_co_cleared", Complete: true}},
		FieldCapabilities: []FieldCapability{{Name: "surf", HMOwned: true}},
	}
	if got := EvaluateGoal(progress, obs); !got.Complete {
		t.Fatalf("progress status = %+v", got)
	}
	if got := EvaluateGoal(capability, obs); !got.Complete {
		t.Fatalf("capability status = %+v", got)
	}
	for _, raw := range []string{"progress:silph_co_cleared", "capability:surf", "field-capability:strength"} {
		if _, structured, err := PlannerGoal(raw); err != nil || !structured {
			t.Fatalf("PlannerGoal(%q) = structured %v err %v", raw, structured, err)
		}
	}
}
