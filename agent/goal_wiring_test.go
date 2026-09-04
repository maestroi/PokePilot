package agent

import "testing"

func TestPlannerGoalDistinguishesDeterministicGoalsFromArbitraryProse(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		structured bool
		wantErr    bool
	}{
		{name: "known Boulder preset is deterministic", raw: "Earn the Boulder Badge.", structured: true},
		{name: "arbitrary prose remains prompt only", raw: "Explore Kanto and see how far you get."},
		{name: "colon in prose remains prompt only", raw: "Goal: earn the Boulder Badge."},
		{name: "elite four", raw: "elite-four", structured: true},
		{name: "badge count", raw: "badges:1", structured: true},
		{name: "malformed structured goal", raw: "badges:99", structured: true, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, structured, err := PlannerGoal(tc.raw)
			if structured != tc.structured {
				t.Fatalf("structured = %v, want %v", structured, tc.structured)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestPlannerGoalStatusUsesObservation(t *testing.T) {
	status, structured, err := PlannerGoalStatus("badges:1", Observation{Badges: []string{"Boulder"}})
	if err != nil {
		t.Fatalf("PlannerGoalStatus: %v", err)
	}
	if !structured || !status.Complete {
		t.Fatalf("structured=%v status=%+v, want completed structured goal", structured, status)
	}

	status, structured, err = PlannerGoalStatus("Earn the Boulder Badge.", Observation{Badges: []string{"Boulder"}})
	if err != nil {
		t.Fatalf("Boulder preset PlannerGoalStatus: %v", err)
	}
	if !structured || !status.Complete || status.Current != 1 || status.Target != 1 {
		t.Fatalf("Boulder preset did not complete deterministically: structured=%v status=%+v", structured, status)
	}

	status, structured, err = PlannerGoalStatus("Explore Kanto and see how far you get.", Observation{Badges: []string{"Boulder"}})
	if err != nil {
		t.Fatalf("free text PlannerGoalStatus: %v", err)
	}
	if structured || status.Complete {
		t.Fatalf("arbitrary free-text goal became deterministic: structured=%v status=%+v", structured, status)
	}
}
