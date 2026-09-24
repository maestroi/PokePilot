package agent

import (
	"reflect"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func TestPrerequisiteRecoverySynthesizesRecoveryOnlyProgressObjective(t *testing.T) {
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = []Prerequisite{{Capability: "can_ride_cycling_road"}}

	obs := Observation{RouteBlockages: []RouteBlockage{{
		Destination: "fuchsia city",
		Missing:     []CapabilityID{"can_ride_cycling_road"},
		Prerequisites: []RoutePrerequisiteLink{{
			Capability:   "can_ride_cycling_road",
			Progress:     redProgressBicycleAcquired,
			RecoveryOnly: true,
		}},
	}}}
	want := Objective{Kind: KindProgress, Progress: redProgressBicycleAcquired}
	offered := []Objective{{Kind: KindGoTo, Place: "cerulean city"}}

	got, capabilities, ok := policy.prerequisiteRecovery(obs, offered)
	if !ok {
		t.Fatal("recovery-only Bicycle prerequisite did not trigger deterministic recovery")
	}
	if got.Key() != want.Key() {
		t.Fatalf("recovery objective = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(capabilities, []Prerequisite{{Capability: "can_ride_cycling_road"}}) {
		t.Fatalf("recovery capabilities = %v", capabilities)
	}
}

func TestPrerequisiteRecoveryStillRequiresNormalOfferWithoutRecoveryOnly(t *testing.T) {
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = []Prerequisite{{Capability: "can_enter_saffron"}}
	obs := Observation{RouteBlockages: []RouteBlockage{{
		Destination: "saffron city",
		Missing:     []CapabilityID{"can_enter_saffron"},
		Prerequisites: []RoutePrerequisiteLink{{
			Capability: "can_enter_saffron",
			Progress:   ProgressSaffronGateOpen,
		}},
	}}}
	if got, capabilities, ok := policy.prerequisiteRecovery(obs, []Objective{{Kind: KindGoTo, Place: "celadon city"}}); ok {
		t.Fatalf("non-recovery-only prerequisite synthesized unavailable objective %+v via %v", got, capabilities)
	}
}

func TestPrerequisiteRecoverySynthesizesUnlockedFieldCapabilityRepair(t *testing.T) {
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = []Prerequisite{{Capability: "can_surf"}}

	obs := Observation{
		FieldCapabilities: []FieldCapability{{
			Name:       "surf",
			BadgeOwned: true,
			HMOwned:    true,
			Usable:     false,
		}},
		RouteBlockages: []RouteBlockage{{
			Destination: "cinnabar island",
			Missing:     []CapabilityID{"can_surf"},
			Prerequisites: []RoutePrerequisiteLink{{
				Capability:      "can_surf",
				FieldCapability: "surf",
			}},
		}},
	}
	got, capabilities, ok := policy.prerequisiteRecovery(obs, []Objective{{Kind: KindCatch, Species: "pidgey"}})
	if !ok {
		t.Fatal("Surf route prerequisite did not trigger deterministic field-capability recovery")
	}
	want := Objective{Kind: KindRepairFieldCapability, FieldCapability: "surf"}
	if got.Key() != want.Key() {
		t.Fatalf("recovery objective = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(capabilities, []Prerequisite{{Capability: "can_surf"}}) {
		t.Fatalf("recovery capabilities = %v", capabilities)
	}
}

func TestPrerequisiteRecoveryDoesNotRepairLockedFieldCapability(t *testing.T) {
	for _, field := range []FieldCapability{
		{Name: "surf", BadgeOwned: false, HMOwned: true},
		{Name: "surf", BadgeOwned: true, HMOwned: false},
		{Name: "surf", BadgeOwned: true, HMOwned: true, Usable: true},
	} {
		policy := newRunFailurePolicy(3)
		policy.pendingPrerequisites = []Prerequisite{{Capability: "can_surf"}}
		obs := Observation{
			FieldCapabilities: []FieldCapability{field},
			RouteBlockages: []RouteBlockage{{
				Destination: "cinnabar island",
				Missing:     []CapabilityID{"can_surf"},
				Prerequisites: []RoutePrerequisiteLink{{
					Capability:      "can_surf",
					FieldCapability: "surf",
				}},
			}},
		}
		if got, capabilities, ok := policy.prerequisiteRecovery(obs, []Objective{{Kind: KindCatch, Species: "pidgey"}}); ok {
			t.Fatalf("locked/already-usable field capability %+v synthesized recovery %+v via %v", field, got, capabilities)
		}
	}
}

func TestPrerequisiteRecoveryPrefersOfferedProgressBeforeFieldRepair(t *testing.T) {
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = []Prerequisite{{Capability: "can_cut"}}
	obs := Observation{
		FieldCapabilities: []FieldCapability{{
			Name:       "cut",
			BadgeOwned: true,
			HMOwned:    true,
		}},
		RouteBlockages: []RouteBlockage{{
			Destination: "vermilion gym",
			Missing:     []CapabilityID{"can_cut"},
			Prerequisites: []RoutePrerequisiteLink{{
				Capability:      "can_cut",
				FieldCapability: "cut",
				Progress:        redProgressHM01Acquired,
			}},
		}},
	}
	progress := Objective{Kind: KindProgress, Progress: redProgressHM01Acquired}
	got, _, ok := policy.prerequisiteRecovery(obs, []Objective{progress})
	if !ok || got.Key() != progress.Key() {
		t.Fatalf("recovery = %+v, ok=%v; want offered progression %+v", got, ok, progress)
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
	want := []Prerequisite{{Capability: "can_clear_snorlax"}, {Capability: "can_ride_cycling_road"}}
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
	policy.pendingPrerequisites = []Prerequisite{{Capability: "can_ride_cycling_road"}}
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
