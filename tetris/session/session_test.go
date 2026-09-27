package session

import (
	"testing"

	"github.com/maestroi/pokepilot/tetris"
	"github.com/maestroi/pokepilot/tetris/policy"
)

func TestParseGoal(t *testing.T) {
	tests := []struct {
		raw       string
		kind      GoalKind
		target    int
		objective policy.Objective
		mode      tetris.Mode
	}{
		{"", GoalAuto, 0, policy.ObjectiveAuto, tetris.ModeA},
		{"auto", GoalAuto, 0, policy.ObjectiveAuto, tetris.ModeA},
		{"survival", GoalSurvival, 0, policy.ObjectiveSurvival, tetris.ModeA},
		{"score:10000", GoalScore, 10000, policy.ObjectiveScore, tetris.ModeA},
		{"lines:25", GoalLines, 25, policy.ObjectiveLines, tetris.ModeB},
		{"complete", GoalComplete, 0, policy.ObjectiveAuto, tetris.ModeB},
	}
	for _, tc := range tests {
		got, err := ParseGoal(tc.raw)
		if err != nil {
			t.Fatalf("ParseGoal(%q): %v", tc.raw, err)
		}
		if got.Kind != tc.kind || got.Target != tc.target || got.Objective != tc.objective || got.Mode() != tc.mode {
			t.Fatalf("ParseGoal(%q) = %#v mode=%s", tc.raw, got, got.Mode())
		}
	}
}

func TestParseGoalRejectsInvalidTargets(t *testing.T) {
	for _, raw := range []string{"score:0", "score:nope", "lines:-1", "badges:1"} {
		if _, err := ParseGoal(raw); err == nil {
			t.Fatalf("ParseGoal(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestGoalSatisfied(t *testing.T) {
	score := Goal{Kind: GoalScore, Target: 1000, Objective: policy.ObjectiveScore}
	if score.Satisfied(tetris.State{ScoreValid: true, Score: 999}) {
		t.Fatal("score goal completed early")
	}
	if !score.Satisfied(tetris.State{ScoreValid: true, Score: 1000}) {
		t.Fatal("score goal did not complete at target")
	}

	lines := Goal{Kind: GoalLines, Target: 25, Objective: policy.ObjectiveLines}
	if !lines.Satisfied(tetris.State{LinesCleared: 25}) {
		t.Fatal("line goal did not complete at target")
	}
	if !lines.Satisfied(tetris.State{Mode: tetris.ModeB, Complete: true}) {
		t.Fatal("Type B completion did not satisfy line goal")
	}

	complete := Goal{Kind: GoalComplete, Objective: policy.ObjectiveAuto}
	if !complete.Satisfied(tetris.State{Complete: true}) {
		t.Fatal("complete goal did not recognize completion")
	}
}
