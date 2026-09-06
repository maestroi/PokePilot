package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestFocusWalkingJourneysKeepsCurrentAndAdjacentMaps(t *testing.T) {
	current := mustPlaceForTravelFocusTest(t, "route 1")
	adjacent := mustPlaceForTravelFocusTest(t, "viridian city")

	known := NewKnowledge(map[uint8][]uint8{
		current.Map: {adjacent.Map},
	})
	obs := Observation{Map: current.Map}
	offered := []Objective{
		{Kind: KindGoTo, Place: "route 1"},
		{Kind: KindGoTo, Place: "viridian city"},
		{Kind: KindGoTo, Place: "viridian city", Flee: true},
		{Kind: KindGoTo, Place: "reds bedroom"},
		{Kind: KindGoTo, Place: "reds bedroom", Flee: true},
	}

	got := focusWalkingJourneys(obs, known, offered)
	if len(got) != 3 {
		t.Fatalf("focused travel has %d choices, want 3: %#v", len(got), got)
	}
	for _, objective := range got {
		if objective.Place == "reds bedroom" {
			t.Fatalf("distant remembered destination survived local travel filter: %v", objective)
		}
	}
}

func TestFocusWalkingJourneysPreservesReasonedLongDistanceObjectives(t *testing.T) {
	current := mustPlaceForTravelFocusTest(t, "route 1")
	known := NewKnowledge(nil)
	obs := Observation{Map: current.Map}
	offered := []Objective{
		{Kind: KindGoTo, Place: "reds bedroom"},
		{Kind: KindHeal, Place: "viridian pokemon center", Flee: true},
		{Kind: KindErrand},
	}

	got := focusWalkingJourneys(obs, known, offered)
	if len(got) != 2 {
		t.Fatalf("focused menu has %d choices, want 2: %#v", len(got), got)
	}
	if got[0].Kind != KindHeal || got[1].Kind != KindErrand {
		t.Fatalf("reasoned long-distance objectives changed: %#v", got)
	}
}

func mustPlaceForTravelFocusTest(t *testing.T, name string) skill.Destination {
	t.Helper()
	destination, ok := skill.Place(name)
	if !ok {
		t.Fatalf("test place %q missing from skill table", name)
	}
	return destination
}
