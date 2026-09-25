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
		{style: "completionist", want: DefaultEliteFourGoal},
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
	completionist := Spec{RunID: "default-completionist", Planner: "llm", PlayStyle: "completionist"}
	ApplyPlayStyleDefaultGoal(&completionist)
	if completionist.Goal.String() != DefaultEliteFourGoal {
		t.Fatalf("completionist default goal = %q, want %q", completionist.Goal.String(), DefaultEliteFourGoal)
	}

	explicit := Spec{RunID: "explicit-goal", Planner: "llm", PlayStyle: "completionist", Goal: GoalFrom("badges:3")}
	ApplyPlayStyleDefaultGoal(&explicit)
	if explicit.Goal.String() != "badges:3" {
		t.Fatalf("explicit goal overwritten: got %q", explicit.Goal.String())
	}

	freePlay := Spec{RunID: "explicit-free-play", Planner: "llm", PlayStyle: "completionist", Goal: GoalFrom("")}
	ApplyPlayStyleDefaultGoal(&freePlay)
	if freePlay.Goal.String() != "" || !freePlay.Goal.Provided() {
		t.Fatalf("explicit free-play goal was replaced: %q provided=%v", freePlay.Goal.String(), freePlay.Goal.Provided())
	}

	legacy := Spec{RunID: "legacy-no-style", Planner: "llm"}
	ApplyPlayStyleDefaultGoal(&legacy)
	if legacy.Goal.Provided() {
		t.Fatalf("legacy spec acquired goal %q", legacy.Goal.String())
	}

	scripted := Spec{RunID: "scripted-style", Planner: "scripted", PlayStyle: "completionist"}
	ApplyPlayStyleDefaultGoal(&scripted)
	if scripted.Goal.Provided() {
		t.Fatalf("scripted spec acquired goal %q", scripted.Goal.String())
	}
}

// TestSpecDecodeKeepsWireFaithful pins the rule that decoding a Spec is a
// faithful record of what the operator asked for: the play-style default goal
// is applied by the runner when it starts a run, not by the decoder.
func TestSpecDecodeKeepsWireFaithful(t *testing.T) {
	var completionist Spec
	if err := json.Unmarshal([]byte(`{"run_id":"decoded-completionist","planner":"llm","play_style":"completionist"}`), &completionist); err != nil {
		t.Fatal(err)
	}
	if completionist.PlayStyle != "completionist" {
		t.Fatalf("decoded play style = %q, want completionist", completionist.PlayStyle)
	}
	if completionist.Goal.Provided() {
		t.Fatalf("decode invented a goal %q", completionist.Goal.String())
	}
	ApplyPlayStyleDefaultGoal(&completionist)
	if completionist.Goal.String() != DefaultEliteFourGoal {
		t.Fatalf("resolved completionist goal = %q, want %q", completionist.Goal.String(), DefaultEliteFourGoal)
	}

	var explicit Spec
	if err := json.Unmarshal([]byte(`{"run_id":"decoded-explicit","planner":"llm","play_style":"completionist","goal":"badges:4"}`), &explicit); err != nil {
		t.Fatal(err)
	}
	ApplyPlayStyleDefaultGoal(&explicit)
	if explicit.Goal.String() != "badges:4" {
		t.Fatalf("decoded explicit goal = %q, want badges:4", explicit.Goal.String())
	}
}

func TestSpecWirePreservesExplicitFreePlay(t *testing.T) {
	var first Spec
	if err := json.Unmarshal([]byte(`{"run_id":"decoded-free-play","planner":"llm","play_style":"completionist","goal":""}`), &first); err != nil {
		t.Fatal(err)
	}
	if first.Goal.String() != "" || !first.Goal.Provided() {
		t.Fatalf("explicit free-play goal = %q provided=%v, want provided empty", first.Goal.String(), first.Goal.Provided())
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
	if leased.Goal.String() != "" || !leased.Goal.Provided() {
		t.Fatalf("leased free-play goal = %q provided=%v, want provided empty", leased.Goal.String(), leased.Goal.Provided())
	}
}
