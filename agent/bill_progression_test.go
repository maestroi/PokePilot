package agent

import "testing"

func TestBillProgressionObjectiveLifecycle(t *testing.T) {
	if !redProgressionKnown(redProgressSSTicketAcquired) {
		t.Fatalf("Bill progress %q is not registered", redProgressSSTicketAcquired)
	}

	obs := Observation{Map: 0x41}
	found := false
	for _, objective := range redProgressionObjectives(obs) {
		if objective.Progress == redProgressSSTicketAcquired {
			found = true
			if objective.Kind != KindProgress {
				t.Fatalf("Bill objective kind = %q, want %q", objective.Kind, KindProgress)
			}
		}
	}
	if !found {
		t.Fatal("Bill objective not offered immediately after Misty in Cerulean Gym")
	}

	obs.Story = ProgressState{{ID: redProgressSSTicketAcquired, Complete: true}}
	for _, objective := range redProgressionObjectives(obs) {
		if objective.Progress == redProgressSSTicketAcquired {
			t.Fatalf("completed Bill objective was offered again: %+v", objective)
		}
	}
}

func TestCeruleanRobbedHousePrerequisiteLinksToBill(t *testing.T) {
	link, ok := redRoutePrerequisiteLink("can_pass_cerulean_robbed_house")
	if !ok {
		t.Fatal("Cerulean robbed-house capability has no planner prerequisite link")
	}
	if link.Progress != redProgressSSTicketAcquired {
		t.Fatalf("robbed-house prerequisite progress = %q, want %q", link.Progress, redProgressSSTicketAcquired)
	}
}
