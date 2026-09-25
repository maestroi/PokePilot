package farm

import (
	"encoding/json"
	"testing"
)

// TestRunGoalTriState pins the three states a run goal can be in. They are not
// interchangeable: an unset goal may still receive a play-style default, while
// an explicit empty goal means Free play and must never acquire one.
func TestRunGoalTriState(t *testing.T) {
	var zero RunGoal
	if zero.Provided() || zero.String() != "" {
		t.Fatalf("zero RunGoal = %q provided=%v, want unset", zero.String(), zero.Provided())
	}

	freePlay := GoalFrom("")
	if !freePlay.Provided() || freePlay.String() != "" {
		t.Fatalf("GoalFrom(\"\") = %q provided=%v, want provided empty", freePlay.String(), freePlay.Provided())
	}

	explicit := GoalFrom("badges:3")
	if !explicit.Provided() || explicit.String() != "badges:3" {
		t.Fatalf("GoalFrom(badges:3) = %q provided=%v", explicit.String(), explicit.Provided())
	}
}

// TestRunGoalJSONDistinguishesAbsentFromEmpty covers the wire form: an unset
// goal is omitted entirely, while a provided goal is always a JSON string.
func TestRunGoalJSONDistinguishesAbsentFromEmpty(t *testing.T) {
	type holder struct {
		Goal RunGoal `json:"goal,omitzero"`
	}

	unset, err := json.Marshal(holder{})
	if err != nil {
		t.Fatal(err)
	}
	if string(unset) != `{}` {
		t.Fatalf("unset goal wire = %s, want {}", unset)
	}

	freePlay, err := json.Marshal(holder{Goal: GoalFrom("")})
	if err != nil {
		t.Fatal(err)
	}
	if string(freePlay) != `{"goal":""}` {
		t.Fatalf("free play goal wire = %s, want explicit empty string", freePlay)
	}

	var decoded holder
	if err := json.Unmarshal([]byte(`{"goal":"badges:3"}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Goal.Provided() || decoded.Goal.String() != "badges:3" {
		t.Fatalf("decoded goal = %q provided=%v", decoded.Goal.String(), decoded.Goal.Provided())
	}
}

// TestHeldGoalReportsProvidedNotResolved pins that HeldGoal answers "did the
// operator supply a goal", not "is a goal known": a style with a pending
// default is still unresolved at this point.
func TestHeldGoalReportsProvidedNotResolved(t *testing.T) {
	goal, provided := HeldGoal(Spec{RunID: "no-style"})
	if provided || goal != "" {
		t.Fatalf("HeldGoal(no style) = %q provided=%v, want unset", goal, provided)
	}

	goal, provided = HeldGoal(Spec{RunID: "unknown-style", PlayStyle: "mystery"})
	if provided || goal != "" {
		t.Fatalf("HeldGoal(unknown style) = %q provided=%v, want unset", goal, provided)
	}
}

// TestHeldGoalFreePlaySurvivesADefaultingStyle is the reason the tri-state
// exists: an explicit Free play goal must not be replaced by the play-style
// default on the next lease.
func TestHeldGoalFreePlaySurvivesADefaultingStyle(t *testing.T) {
	goal, provided := HeldGoal(Spec{RunID: "free-play", PlayStyle: "completionist", Goal: GoalFrom("")})
	if !provided || goal != "" {
		t.Fatalf("HeldGoal(free play) = %q provided=%v, want provided empty", goal, provided)
	}
}

// TestHeldGoalNeverInventsADefault pins that the play-style default is applied
// by the runner at start time, not baked into the held goal by the projection.
func TestHeldGoalNeverInventsADefault(t *testing.T) {
	goal, provided := HeldGoal(Spec{RunID: "completionist", PlayStyle: "completionist"})
	if provided || goal != "" {
		t.Fatalf("HeldGoal(pending style) = %q provided=%v, want unresolved", goal, provided)
	}

	spec := Spec{RunID: "completionist", Planner: "llm", PlayStyle: "completionist"}
	ApplyPlayStyleDefaultGoal(&spec)
	goal, provided = HeldGoal(spec)
	if !provided || goal != DefaultEliteFourGoal {
		t.Fatalf("HeldGoal(resolved style) = %q provided=%v, want %q", goal, provided, DefaultEliteFourGoal)
	}
}
