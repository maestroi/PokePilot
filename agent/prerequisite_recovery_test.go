package agent

import (
	"reflect"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func TestPrerequisiteRecoveryChoosesLinkedProgressObjective(t *testing.T) {
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = []CapabilityID{"can_ride_cycling_road"}

	obs := Observation{RouteBlockages: []RouteBlockage{{
		Destination: "fuchsia city",
		Missing:     []CapabilityID{"can_ride_cycling_road"},
		Prerequisites: []RoutePrerequisiteLink{{
			Capability: "can_ride_cycling_road",
			Progress:   redProgressBicycleAcquired,
		}},
	}}}
	want := Objective{Kind: KindProgress, Progress: redProgressBicycleAcquired}
	offered := []Objective{
		{Kind: KindGoTo, Place: "cerulean city"},
		want,
	}

	got, capabilities, ok := policy.prerequisiteRecovery(obs, offered)
	if !ok {
		t.Fatal("linked Bicycle prerequisite did not trigger deterministic recovery")
	}
	if got.Key() != want.Key() {
		t.Fatalf("recovery objective = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(capabilities, []CapabilityID{"can_ride_cycling_road"}) {
		t.Fatalf("recovery capabilities = %v", capabilities)
	}
}

func TestPrerequisiteRecoveryDoesNotGuessFieldCapabilityRepair(t *testing.T) {
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = []CapabilityID{"can_surf"}

	obs := Observation{RouteBlockages: []RouteBlockage{{
		Destination: "cinnabar island",
		Missing:     []CapabilityID{"can_surf"},
		Prerequisites: []RoutePrerequisiteLink{{
			Capability:      "can_surf",
			FieldCapability: "surf",
		}},
	}}}
	offered := []Objective{{Kind: KindProgress, Progress: ProgressID("unrelated")}}
	if got, capabilities, ok := policy.prerequisiteRecovery(obs, offered); ok {
		t.Fatalf("guessed recovery = %+v via %v; field-move preparation has no explicit progression link", got, capabilities)
	}
}

func TestFailurePolicyRecordsMissingRouteCapabilitiesForRecovery(t *testing.T) {
	policy := newRunFailurePolicy(3)
	result := ObjectiveResult{
		Objective: Objective{Kind: KindGoTo, Place: "fuchsia city"},
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "route_prerequisite_missing",
			Context:     []string{"can_clear_snorlax", "can_ride_cycling_road"},
			Recoverable: true,
		},
		Final: Observation{},
	}
	policy.record(result)
	want := []CapabilityID{"can_clear_snorlax", "can_ride_cycling_road"}
	if !reflect.DeepEqual(policy.pendingPrerequisites, want) {
		t.Fatalf("pending prerequisites = %v, want %v", policy.pendingPrerequisites, want)
	}

	policy.success()
	if len(policy.pendingPrerequisites) != 0 {
		t.Fatalf("successful recovery left stale prerequisites: %v", policy.pendingPrerequisites)
	}
}

func TestNonPrerequisiteFailureClearsPendingRouteRecovery(t *testing.T) {
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = []CapabilityID{"can_ride_cycling_road"}
	result := ObjectiveResult{
		Objective: Objective{Kind: KindGoTo, Place: "route 9"},
		Outcome:   OutcomeBlocked,
		Failure: &gameruntime.Failure{
			Class:       gameruntime.FailureClassBlocked,
			Cause:       "navigation_stalled",
			Recoverable: true,
		},
		Final: Observation{},
	}
	policy.record(result)
	if len(policy.pendingPrerequisites) != 0 {
		t.Fatalf("non-prerequisite failure retained stale recovery: %v", policy.pendingPrerequisites)
	}
}
