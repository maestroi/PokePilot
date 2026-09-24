package agent

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestBoulderProgressionIsExplicitAfterPokedex(t *testing.T) {
	obs := Observation{
		Story: ProgressState{{ID: redProgressPokedexAcquired, Complete: true}},
	}
	got := redProgressionObjectives(obs)
	if !hasProgressObjective(got, redProgressBoulderBadge) {
		t.Fatalf("post-Pokedex state did not offer Boulder progression: %v", got)
	}

	obs.Badges = []string{state.BadgeBoulder.String()}
	obs.Story = append(obs.Story, ProgressFact{ID: redProgressBoulderBadge, Complete: true})
	if hasProgressObjective(redProgressionObjectives(obs), redProgressBoulderBadge) {
		t.Fatal("Boulder progression remained offered after badge acquisition")
	}
}

func TestPewterExitCapabilityLinksToBoulderProgression(t *testing.T) {
	link, ok := redRoutePrerequisiteLink("can_leave_pewter_east")
	if !ok {
		t.Fatal("can_leave_pewter_east has no Red prerequisite link")
	}
	if link.Badge != state.BadgeBoulder.String() || link.Progress != redProgressBoulderBadge {
		t.Fatalf("Pewter prerequisite = %+v, want Boulder badge + %q", link, redProgressBoulderBadge)
	}
}

func TestPrerequisiteRecoveryChoosesBrockForPewterExit(t *testing.T) {
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = []Prerequisite{{Capability: "can_leave_pewter_east"}}

	obs := Observation{
		Story: ProgressState{{ID: redProgressPokedexAcquired, Complete: true}},
		RouteBlockages: []RouteBlockage{{
			Destination: "route 3",
			Missing:     []Prerequisite{{Capability: "can_leave_pewter_east"}},
			Prerequisites: []RoutePrerequisiteLink{{
				Capability: "can_leave_pewter_east",
				Badge:      state.BadgeBoulder.String(),
				Progress:   redProgressBoulderBadge,
			}},
		}},
	}
	want := Objective{Kind: KindProgress, Progress: redProgressBoulderBadge}
	offered := redProgressionObjectives(obs)
	got, capabilities, ok := policy.prerequisiteRecovery(obs, offered)
	if !ok {
		t.Fatal("Pewter exit blockage did not trigger deterministic Brock recovery")
	}
	if got.Key() != want.Key() {
		t.Fatalf("recovery objective = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(capabilities, []CapabilityID{"can_leave_pewter_east"}) {
		t.Fatalf("recovery capabilities = %v", capabilities)
	}
}
