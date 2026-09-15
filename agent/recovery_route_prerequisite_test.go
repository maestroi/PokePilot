package agent

import "testing"

func TestRoutePrerequisiteQuarantineCoversTravelPolicySibling(t *testing.T) {
	fleeing := Objective{Kind: KindGoTo, Place: "route 12 super rod house", Flee: true}
	plain := Objective{Kind: KindGoTo, Place: "route 12 super rod house"}
	alternative := Objective{Kind: KindGoTo, Place: "lavender pokemon center"}
	obs := Observation{
		Location:     "lavender town",
		X:            11,
		Y:            20,
		Controllable: true,
	}

	policy := newRunFailurePolicy(3)
	policy.record(ObjectiveResult{
		Objective:    fleeing,
		Outcome:      OutcomeBlocked,
		Cause:        "route_prerequisite_missing",
		CauseContext: []string{"can_clear_snorlax"},
		Final:        obs,
	})

	got := policy.filter(obs, []Objective{plain, fleeing, alternative})
	if len(got) != 1 || got[0].Key() != alternative.Key() {
		t.Fatalf("route prerequisite retry kept an equivalent travel policy variant: %+v", got)
	}
}

func TestNavigationFailureQuarantineKeepsOtherTravelPolicyAvailable(t *testing.T) {
	fleeing := Objective{Kind: KindGoTo, Place: "route 12 super rod house", Flee: true}
	plain := Objective{Kind: KindGoTo, Place: "route 12 super rod house"}
	alternative := Objective{Kind: KindGoTo, Place: "lavender pokemon center"}
	obs := Observation{
		Location:     "lavender town",
		X:            11,
		Y:            20,
		Controllable: true,
	}

	policy := newRunFailurePolicy(3)
	policy.record(ObjectiveResult{
		Objective: fleeing,
		Outcome:   OutcomeBlocked,
		Cause:     "navigation_stalled",
		Final:     obs,
	})

	got := policy.filter(obs, []Objective{plain, fleeing, alternative})
	if len(got) != 2 {
		t.Fatalf("navigation failure over-quarantined travel policies: %+v", got)
	}
	if got[0].Key() != plain.Key() || got[1].Key() != alternative.Key() {
		t.Fatalf("navigation failure removed the wrong travel policies: %+v", got)
	}
}
