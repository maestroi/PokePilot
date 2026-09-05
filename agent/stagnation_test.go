package agent

import "testing"

func TestRoundCapZeroMeansUncapped(t *testing.T) {
	for _, round := range []int{1, 32, 128, 10_000} {
		if roundCapReached(round, 0) {
			t.Fatalf("roundCapReached(%d, 0) = true; zero must be uncapped", round)
		}
		if got := roundsLeft(round, 0); got != 0 {
			t.Fatalf("roundsLeft(%d, 0) = %d, want 0 sentinel for uncapped", round, got)
		}
	}
}

func TestPositiveRoundCapStillWorks(t *testing.T) {
	if roundCapReached(32, 32) {
		t.Fatal("round 32 should still run when MaxRounds is 32")
	}
	if !roundCapReached(33, 32) {
		t.Fatal("round 33 should be beyond an explicit MaxRounds of 32")
	}
	if got := roundsLeft(32, 32); got != 1 {
		t.Fatalf("roundsLeft(32, 32) = %d, want 1", got)
	}
}

func TestMajorProgressMarkOnlyAdvancesOnMeaningfulHighWaterMarks(t *testing.T) {
	mark := majorProgressMark{Badges: 1, Events: 4, Maps: 8, PartyCount: 2, MaxLevel: 15}

	// Pure churn/regression must not buy another watchdog window.
	if mark.absorb(majorProgressMark{Badges: 1, Events: 4, Maps: 8, PartyCount: 1, MaxLevel: 12}) {
		t.Fatal("regression counted as major progress")
	}
	if mark.MaxLevel != 15 || mark.PartyCount != 2 {
		t.Fatalf("high-water mark regressed: %+v", mark)
	}

	// Each monotonic game-progress dimension is allowed to reset stagnation.
	cases := []majorProgressMark{
		{Badges: 2, Events: 4, Maps: 8, PartyCount: 2, MaxLevel: 15},
		{Badges: 2, Events: 5, Maps: 8, PartyCount: 2, MaxLevel: 15},
		{Badges: 2, Events: 5, Maps: 9, PartyCount: 2, MaxLevel: 15},
		{Badges: 2, Events: 5, Maps: 9, PartyCount: 3, MaxLevel: 15},
		{Badges: 2, Events: 5, Maps: 9, PartyCount: 3, MaxLevel: 16},
	}
	for i, next := range cases {
		if !mark.absorb(next) {
			t.Fatalf("case %d did not count as major progress: next=%+v mark=%+v", i, next, mark)
		}
	}
}

func TestMajorProgressMarkIgnoresHPPositionMoneyAndConsumablesByConstruction(t *testing.T) {
	obsA := Observation{
		Map: 1, X: 1, Y: 1, Money: 3000, PartyCount: 1,
		Party: []PartyMon{{Level: 10, HP: 5, MaxHP: 30}},
		Bag:   []Item{{Name: "POTION", Quantity: 8}},
	}
	obsB := Observation{
		Map: 1, X: 18, Y: 22, Money: 25, PartyCount: 1,
		Party: []PartyMon{{Level: 10, HP: 30, MaxHP: 30}},
		Bag:   []Item{{Name: "POTION", Quantity: 1}},
	}
	if a, b := majorProgressMarkOf(obsA, nil), majorProgressMarkOf(obsB, nil); a != b {
		t.Fatalf("churn changed major-progress mark: a=%+v b=%+v", a, b)
	}
}
