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

func TestRedProgressionOffersBicycleAfterHM01(t *testing.T) {
	obs := Observation{Story: ProgressState{{ID: redProgressHM01Acquired, Complete: true}}}
	if got := bicycleProgressCount(redProgressionObjectives(obs)); got != 1 {
		t.Fatalf("bicycle progression offered %d times after HM01, want 1", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: redProgressBicycleAcquired, Complete: true})
	if got := bicycleProgressCount(redProgressionObjectives(obs)); got != 0 {
		t.Fatalf("bicycle progression offered %d times after Bicycle acquired, want 0", got)
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
}

func TestSpeedrunPlannerHidesOptionalBicycleButRecoveryCanStillUseIt(t *testing.T) {
	obs := Observation{Story: ProgressState{{ID: redProgressHM01Acquired, Complete: true}}}
	raw := redProgressionObjectives(obs)
	if bicycleProgressCount(raw) != 1 {
		t.Fatalf("raw progression must keep Bicycle available for typed recovery: %v", raw)
	}

	speed := AnnotatePlayStyle(obs, raw, PlayStyle(PlayStyleSpeedrun))
	if bicycleProgressCount(speed) != 0 {
		t.Fatalf("speedrun planner still sees optional Bicycle progression: %v", speed)
	}

	adventure := AnnotatePlayStyle(obs, raw, PlayStyle(PlayStyleAdventure))
	if bicycleProgressCount(adventure) != 1 {
		t.Fatalf("Adventure should retain optional Bicycle progression: %v", adventure)
	}
}
