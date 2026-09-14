package farm

import (
	"bytes"
	"encoding/json"
	"testing"
)

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

func TestSpecDecodeAppliesPlayStyleDefaultGoal(t *testing.T) {
	var completionist Spec
	if err := json.Unmarshal([]byte(`{"run_id":"decoded-completionist","planner":"llm","play_style":"completionist"}`), &completionist); err != nil {
		t.Fatal(err)
	}
	if completionist.Goal != DefaultDexGoal {
		t.Fatalf("decoded completionist goal = %q, want %q", completionist.Goal, DefaultDexGoal)
	}

	var explicit Spec
	if err := json.Unmarshal([]byte(`{"run_id":"decoded-explicit","planner":"llm","play_style":"completionist","goal":"badges:4"}`), &explicit); err != nil {
		t.Fatal(err)
	}
	if explicit.Goal != "badges:4" {
		t.Fatalf("decoded explicit goal = %q, want badges:4", explicit.Goal)
	}
}

func TestSpecWirePreservesExplicitFreePlay(t *testing.T) {
	var first Spec
	if err := json.Unmarshal([]byte(`{"run_id":"decoded-free-play","planner":"llm","play_style":"completionist","goal":""}`), &first); err != nil {
		t.Fatal(err)
	}
	if first.Goal != "" {
		t.Fatalf("explicit free-play goal = %q, want empty", first.Goal)
	}

	wire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(`"goal":""`)) {
		t.Fatalf("free-play wire = %s, want explicit empty goal", wire)
	}

	var leased Spec
	if err := json.Unmarshal(wire, &leased); err != nil {
		t.Fatal(err)
	}
	if leased.Goal != "" {
		t.Fatalf("leased free-play goal = %q, want empty", leased.Goal)
	}
}
