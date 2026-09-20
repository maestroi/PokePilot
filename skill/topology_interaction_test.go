package skill

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestValidateTopologyInteractionRequiresPairedRouteGoal(t *testing.T) {
	base := topologyInteraction{
		Name:     "test",
		Complete: func(*state.Mem) bool { return false },
		Interact: func() error { return nil },
	}
	if err := validateTopologyInteraction(base); err != nil {
		t.Fatalf("non-route topology interaction rejected: %v", err)
	}

	withGoalNameOnly := base
	withGoalNameOnly.RouteGoal = "door"
	if err := validateTopologyInteraction(withGoalNameOnly); err == nil || !strings.Contains(err.Error(), "supplied together") {
		t.Fatalf("RouteGoal without reachability err=%v, want paired-field error", err)
	}

	withReachabilityOnly := base
	withReachabilityOnly.GoalReachable = func() bool { return false }
	if err := validateTopologyInteraction(withReachabilityOnly); err == nil || !strings.Contains(err.Error(), "supplied together") {
		t.Fatalf("GoalReachable without name err=%v, want paired-field error", err)
	}

	routeScoped := base
	routeScoped.RouteGoal = "selected warp"
	routeScoped.GoalReachable = func() bool { return false }
	if err := validateTopologyInteraction(routeScoped); err != nil {
		t.Fatalf("valid route-scoped interaction rejected: %v", err)
	}
}

func TestValidateTopologyInteractionRequiresVerifiedStateAndAction(t *testing.T) {
	if err := validateTopologyInteraction(topologyInteraction{Name: "missing"}); err == nil {
		t.Fatal("interaction without Complete/Interact was accepted")
	}
}

func TestTopologyInteractionGoalProbeIsDemandDriven(t *testing.T) {
	calls := 0
	spec := topologyInteraction{
		RouteGoal: "selected warp",
		GoalReachable: func() bool {
			calls++
			return true
		},
	}
	if !spec.goalReachable() {
		t.Fatal("reachable route goal reported false")
	}
	if calls != 1 {
		t.Fatalf("goal probe calls=%d, want exactly one live check", calls)
	}
}
