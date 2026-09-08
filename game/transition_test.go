package game

import "testing"

func TestTransitionUnlocksWhenCapabilityAppears(t *testing.T) {
	transition := Transition{
		ID:       "locked_door",
		From:     "room_a",
		To:       "room_b",
		Requires: []CapabilityID{"can_open_door"},
	}

	blocked, ok := EvaluateTransition(transition, NewCapabilitySet())
	if ok {
		t.Fatal("transition usable without required capability")
	}
	if len(blocked.Missing) != 1 || blocked.Missing[0] != CapabilityID("can_open_door") {
		t.Fatalf("missing = %v, want [can_open_door]", blocked.Missing)
	}

	if blocked, ok := EvaluateTransition(transition, NewCapabilitySet("can_open_door")); !ok {
		t.Fatalf("transition remained blocked after capability appeared: %+v", blocked)
	}
}

func TestTransitionKeepsOwnershipSemanticsRepresentable(t *testing.T) {
	transition := Transition{
		ID:             "ticket_gate",
		From:           "street",
		To:             "museum",
		OneWay:         true,
		ChoiceRequired: true,
		Consumes:       []ResourceCost{{Resource: "money", Amount: 50}},
		Effects:        []ProgressID{"museum_entered"},
	}
	if transition.Consumes[0].Resource != ResourceID("money") || transition.Consumes[0].Amount != 50 {
		t.Fatalf("resource cost lost: %+v", transition.Consumes)
	}
	if !transition.OneWay || !transition.ChoiceRequired || transition.Effects[0] != ProgressID("museum_entered") {
		t.Fatalf("transition ownership semantics lost: %+v", transition)
	}
}
