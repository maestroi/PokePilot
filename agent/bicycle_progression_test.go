package agent

import "testing"

func bicycleProgressCount(objectives []Objective) int {
	count := 0
	for _, objective := range objectives {
		if objective.Kind == KindProgress && objective.Progress == redProgressBicycleAcquired {
			count++
		}
	}
	return count
}

func TestRedProgressionDoesNotOfferOptionalBicycleProactively(t *testing.T) {
	obs := Observation{Story: ProgressState{{ID: redProgressHM01Acquired, Complete: true}}}
	if got := bicycleProgressCount(redProgressionObjectives(obs)); got != 0 {
		t.Fatalf("optional Bicycle progression offered %d times without a Cycling Road blockage, want 0", got)
	}
}

func TestBicycleProgressionIsKnown(t *testing.T) {
	if !redProgressionKnown(redProgressBicycleAcquired) {
		t.Fatal("bicycle progression is not registered as a known Red goal")
	}
}

func TestCyclingRoadCapabilityLinksToBicycleProgression(t *testing.T) {
	link, ok := redRoutePrerequisiteLink("can_ride_cycling_road")
	if !ok {
		t.Fatal("can_ride_cycling_road has no Red prerequisite link")
	}
	if link.Capability != "can_ride_cycling_road" {
		t.Fatalf("capability = %q, want can_ride_cycling_road", link.Capability)
	}
	if link.Progress != redProgressBicycleAcquired {
		t.Fatalf("progress = %q, want %q", link.Progress, redProgressBicycleAcquired)
	}
	if !link.RecoveryOnly {
		t.Fatal("Cycling Road Bicycle link must be recovery-only")
	}
}
