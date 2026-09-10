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

func TestHM01ProgressionObjectiveLifecycle(t *testing.T) {
	if !redProgressionKnown(redProgressHM01Acquired) {
		t.Fatalf("HM01 progress %q is not registered", redProgressHM01Acquired)
	}

	obs := Observation{Story: ProgressState{{ID: redProgressSSTicketAcquired, Complete: true}}}
	found := false
	for _, objective := range redProgressionObjectives(obs) {
		if objective.Progress == redProgressHM01Acquired {
			found = true
			if objective.Kind != KindProgress {
				t.Fatalf("HM01 objective kind = %q, want %q", objective.Kind, KindProgress)
			}
		}
	}
	if !found {
		t.Fatal("HM01 objective not offered after S.S. Ticket acquisition")
	}

	obs.Story = append(obs.Story, ProgressFact{ID: redProgressHM01Acquired, Complete: true})
	for _, objective := range redProgressionObjectives(obs) {
		if objective.Progress == redProgressHM01Acquired {
			t.Fatalf("completed HM01 objective was offered again: %+v", objective)
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

func TestSSAnneAndCutPrerequisiteLinks(t *testing.T) {
	ship, ok := redRoutePrerequisiteLink("can_board_ss_anne")
	if !ok {
		t.Fatal("S.S. Anne capability has no planner prerequisite link")
	}
	if ship.Progress != redProgressSSTicketAcquired {
		t.Fatalf("ship prerequisite progress = %q, want %q", ship.Progress, redProgressSSTicketAcquired)
	}

	cut, ok := redRoutePrerequisiteLink("can_cut")
	if !ok {
		t.Fatal("Cut capability has no planner prerequisite link")
	}
	if cut.Progress != redProgressHM01Acquired || cut.FieldCapability != "cut" {
		t.Fatalf("Cut prerequisite = %+v, want HM01 progress + cut field capability", cut)
	}
}
