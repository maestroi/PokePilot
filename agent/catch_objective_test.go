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

func TestCatchObjectivePartySlotOnlyForDirectPartyAdditions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		intent string
		want   bool
	}{
		{name: "wild grass", intent: "", want: false},
		{name: "fishing", intent: dexFishingIntent, want: false},
		{name: "water", intent: dexWaterIntent, want: false},
		{name: "safari", intent: dexSafariIntent, want: false},
		{name: "static capture", intent: dexStaticIntent, want: false},
		{name: "trade replaces member", intent: dexTradeIntent, want: false},
		{name: "gift", intent: dexGiftIntent, want: true},
		{name: "fossil revival", intent: dexFossilIntent, want: true},
		{name: "game corner prize", intent: dexGameCornerIntent, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Objective{Kind: KindCatch, Species: "magikarp", Intent: tc.intent}
			if got := catchObjectiveNeedsPartySlot(o); got != tc.want {
				t.Fatalf("catchObjectiveNeedsPartySlot(%q) = %t, want %t", tc.intent, got, tc.want)
			}
		})
	}
}


func TestCatchObjectiveFrameBudgetReservesControllerCleanup(t *testing.T) {
	catchBudget := objectiveFrameBudgetFor(Objective{Kind: KindCatch, Species: "zubat"})
	if catchBudget != catchObjectiveFrameBudget {
		t.Fatalf("catch objective frame budget = %d, want %d", catchBudget, catchObjectiveFrameBudget)
	}
	if catchBudget <= objectiveFrameBudget {
		t.Fatalf("catch objective frame budget = %d, must exceed ordinary %d-frame watchdog", catchBudget, objectiveFrameBudget)
	}
	if got := objectiveFrameBudgetFor(Objective{Kind: KindGoTo, Place: "mt moon 1f"}); got != objectiveFrameBudget {
		t.Fatalf("ordinary objective frame budget = %d, want %d", got, objectiveFrameBudget)
	}
}
