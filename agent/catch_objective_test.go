package agent

import "testing"

func TestCatchObjectiveOwnsTravelForEveryDexGift(t *testing.T) {
	for _, tc := range []struct {
		species SpeciesID
		place   PlaceID
	}{
		{species: "eevee", place: "celadon mansion eevee"},
		{species: "lapras", place: "silph co lapras"},
		{species: "hitmonlee", place: "fighting dojo hitmonlee"},
		{species: "hitmonchan", place: "fighting dojo hitmonchan"},
	} {
		o := Objective{Kind: KindCatch, Species: tc.species, Place: tc.place, Intent: dexGiftIntent}
		if !catchObjectiveOwnsTravel(o) {
			t.Fatalf("gift objective %+v does not own travel", o)
		}
	}
}

func TestCatchObjectiveKeepsOrdinaryHabitatTravel(t *testing.T) {
	for _, intent := range []string{"", dexFishingIntent, dexWaterIntent} {
		o := Objective{Kind: KindCatch, Species: "magikarp", Place: "route 12", Intent: intent}
		if catchObjectiveOwnsTravel(o) {
			t.Fatalf("ordinary habitat objective %+v unexpectedly owns travel", o)
		}
	}
}
