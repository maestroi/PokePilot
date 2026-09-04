package agent

import "testing"

func TestPlannerGoalPresetBoulderStopsOnFirstBadge(t *testing.T) {
	status, structured, err := PlannerGoalStatus("Earn the Boulder Badge.", Observation{Badges: []string{"Boulder"}})
	if err != nil {
		t.Fatalf("PlannerGoalStatus: %v", err)
	}
	if !structured {
		t.Fatal("Boulder preset remained prompt-only")
	}
	if !status.Complete || status.Current != 1 || status.Target != 1 {
		t.Fatalf("Boulder preset status = %+v", status)
	}
}

func TestPlannerGoalPresetMilestones(t *testing.T) {
	cases := []struct {
		goal string
		want Goal
	}{
		{"Earn 2 badges.", Goal{Kind: GoalBadges, Count: 2}},
		{"Earn all 8 badges.", Goal{Kind: GoalBadges, Count: 8}},
		{"Beat the Elite Four and Champion.", Goal{Kind: GoalEliteFour}},
	}
	for _, tc := range cases {
		got, structured, err := PlannerGoal(tc.goal)
		if err != nil {
			t.Fatalf("PlannerGoal(%q): %v", tc.goal, err)
		}
		if !structured {
			t.Fatalf("PlannerGoal(%q) remained prompt-only", tc.goal)
		}
		if got != tc.want {
			t.Fatalf("PlannerGoal(%q) = %+v, want %+v", tc.goal, got, tc.want)
		}
	}
}

func TestPlannerGoalArbitraryProseRemainsPromptOnly(t *testing.T) {
	if _, structured, err := PlannerGoal("Go explore and see how far you get."); err != nil {
		t.Fatalf("PlannerGoal: %v", err)
	} else if structured {
		t.Fatal("arbitrary prose unexpectedly became a deterministic goal")
	}
}
