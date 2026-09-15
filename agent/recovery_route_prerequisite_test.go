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

func TestRoutePrerequisiteQuarantineSurvivesPositionDrift(t *testing.T) {
	fleeing := Objective{Kind: KindGoTo, Place: "pewter city", Flee: true}
	plain := Objective{Kind: KindGoTo, Place: "pewter city"}
	alternative := Objective{Kind: KindGoTo, Place: "route 10"}
	obs := Observation{
		Location:     "route 9",
		X:            59,
		Y:            9,
		Controllable: true,
	}

	policy := newRunFailurePolicy(3)
	policy.record(ObjectiveResult{
		Objective:    fleeing,
		Outcome:      OutcomeBlocked,
		Cause:        "route_prerequisite_missing",
		CauseContext: []string{"can_enter_saffron"},
		Final:        obs,
	})

	// Unrelated movement must not make a missing portable capability look fixed.
	// This is the recurrence from #579: another objective can move Red before
	// the planner sees Pewter again, while can_enter_saffron is still absent.
	moved := obs
	moved.Location = "route 10"
	moved.X, moved.Y = 11, 20
	got := policy.filter(moved, []Objective{plain, fleeing, alternative})
	if len(got) != 1 || got[0].Key() != alternative.Key() {
		t.Fatalf("position drift reopened route-prerequisite failure: %+v", got)
	}
}

func TestRoutePrerequisiteQuarantineExpiresAfterSemanticProgress(t *testing.T) {
	fleeing := Objective{Kind: KindGoTo, Place: "pewter city", Flee: true}
	plain := Objective{Kind: KindGoTo, Place: "pewter city"}
	alternative := Objective{Kind: KindGoTo, Place: "route 10"}
	obs := Observation{
		Location:     "route 9",
		X:            59,
		Y:            9,
		Controllable: true,
	}

	policy := newRunFailurePolicy(3)
	policy.record(ObjectiveResult{
		Objective:    fleeing,
		Outcome:      OutcomeBlocked,
		Cause:        "route_prerequisite_missing",
		CauseContext: []string{"can_enter_saffron"},
		Final:        obs,
	})

	progressed := obs
	progressed.Story = ProgressState{{ID: ProgressSaffronGateOpen, Complete: true}}
	got := policy.filter(progressed, []Objective{plain, fleeing, alternative})
	if len(got) != 3 {
		t.Fatalf("semantic route progress did not release quarantine: %+v", got)
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

func TestNavigationFailureQuarantineStillExpiresAfterPositionChange(t *testing.T) {
	failed := Objective{Kind: KindGoTo, Place: "route 12 super rod house", Flee: true}
	alternative := Objective{Kind: KindGoTo, Place: "lavender pokemon center"}
	obs := Observation{
		Location:     "lavender town",
		X:            11,
		Y:            20,
		Controllable: true,
	}

	policy := newRunFailurePolicy(3)
	policy.record(ObjectiveResult{
		Objective: failed,
		Outcome:   OutcomeBlocked,
		Cause:     "navigation_stalled",
		Final:     obs,
	})

	moved := obs
	moved.X++
	got := policy.filter(moved, []Objective{failed, alternative})
	if len(got) != 2 || got[0].Key() != failed.Key() {
		t.Fatalf("position change should release ordinary navigation quarantine: %+v", got)
	}
}
