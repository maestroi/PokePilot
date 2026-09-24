package agent

import "testing"

func TestRedSafariTrainingMapOnlyAcceptsOutdoorHabitats(t *testing.T) {
	for _, mapID := range []uint8{safariZoneEastMap, safariZoneNorthMap, safariZoneWestMap, safariZoneCenterMap} {
		if !redSafariTrainingMap(mapID) {
			t.Fatalf("Safari habitat %#04x rejected", mapID)
		}
	}
	for _, mapID := range []uint8{safariZoneGateMap, safariZoneCenterRestHouseMap, fuchsiaCityMap} {
		if redSafariTrainingMap(mapID) {
			t.Fatalf("non-habitat map %#04x accepted as Safari training map", mapID)
		}
	}
}

func TestFilterRedSafariTrainingObjectivesSuppressesOnlyTraining(t *testing.T) {
	in := []Objective{
		{Kind: KindTrain, Level: 35},
		{Kind: KindProgress, Progress: "fuchsia_progression_complete"},
		{Kind: KindCatch, Species: "TAUROS"},
	}
	got := filterRedSafariTrainingObjectives(Observation{Map: safariZoneCenterMap}, append([]Objective(nil), in...))
	if len(got) != 2 {
		t.Fatalf("Safari offer = %+v, want two non-training objectives", got)
	}
	for _, objective := range got {
		if objective.Kind == KindTrain {
			t.Fatalf("Safari offer retained training objective: %+v", got)
		}
	}

	outside := filterRedSafariTrainingObjectives(Observation{Map: fuchsiaCityMap}, append([]Objective(nil), in...))
	if len(outside) != len(in) {
		t.Fatalf("outside Safari offer = %+v, want unchanged %+v", outside, in)
	}
}
