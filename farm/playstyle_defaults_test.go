package farm

import "testing"

func TestDefaultGoalForPlayStyle(t *testing.T) {
	for _, tc := range []struct {
		style string
		want  string
	}{
		{style: "speedrun", want: DefaultEliteFourGoal},
		{style: "adventure", want: DefaultEliteFourGoal},
		{style: "completionist", want: DefaultDexGoal},
		{style: "team_builder", want: DefaultEliteFourGoal},
		{style: "team-builder", want: DefaultEliteFourGoal},
		{style: "", want: ""},
		{style: "unknown", want: ""},
	} {
		if got := DefaultGoalForPlayStyle(tc.style); got != tc.want {
			t.Fatalf("DefaultGoalForPlayStyle(%q) = %q, want %q", tc.style, got, tc.want)
		}
	}
}

func TestApplyPlayStyleDefaultGoal(t *testing.T) {
	completionist := Spec{RunID: "default-completionist", Planner: "llm"}
	RememberPlayStyle(completionist.RunID, "completionist")
	ApplyPlayStyleDefaultGoal(&completionist)
	if completionist.Goal != DefaultDexGoal {
		t.Fatalf("completionist default goal = %q, want %q", completionist.Goal, DefaultDexGoal)
	}

	explicit := Spec{RunID: "explicit-goal", Planner: "llm", Goal: "badges:3"}
	RememberPlayStyle(explicit.RunID, "completionist")
	ApplyPlayStyleDefaultGoal(&explicit)
	if explicit.Goal != "badges:3" {
		t.Fatalf("explicit goal overwritten: got %q", explicit.Goal)
	}

	legacy := Spec{RunID: "legacy-no-style", Planner: "llm"}
	ApplyPlayStyleDefaultGoal(&legacy)
	if legacy.Goal != "" {
		t.Fatalf("legacy spec acquired goal %q", legacy.Goal)
	}

	scripted := Spec{RunID: "scripted-style", Planner: "scripted"}
	RememberPlayStyle(scripted.RunID, "completionist")
	ApplyPlayStyleDefaultGoal(&scripted)
	if scripted.Goal != "" {
		t.Fatalf("scripted spec acquired goal %q", scripted.Goal)
	}
}
