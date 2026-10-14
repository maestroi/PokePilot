package game

import "testing"

func TestPlanSpeciesClocksWaitsUntilMorning(t *testing.T) {
	// Pidgey is morning and day. Sentret is morning only. Rattata is every
	// period. Magikarp is an ungated water slot plus a morning grass slot,
	// so it can be met without waiting.
	slots := []TimedSlot{
		{Species: "sentret", Level: 3, Times: []TimeOfDay{TimeMorning}},
		{Species: "pidgey", Level: 4, Times: []TimeOfDay{TimeDay}},
		{Species: "pidgey", Level: 2, Times: []TimeOfDay{TimeMorning}},
		{Species: "pidgey", Level: 3, Times: []TimeOfDay{TimeMorning}},
		{Species: "rattata", Level: 4, Times: []TimeOfDay{TimeMorning}},
		{Species: "rattata", Level: 2, Times: []TimeOfDay{TimeDay}},
		{Species: "rattata", Level: 3, Times: []TimeOfDay{TimeNight}},
		{Species: "magikarp", Level: 15},
		{Species: "magikarp", Level: 5, Times: []TimeOfDay{TimeMorning}},
	}

	night := PlanSpeciesClocks(TimeNight, slots)
	pidgey := speciesClock(t, night, "pidgey")
	if pidgey.Open || pidgey.Wait == nil || pidgey.Wait.Until != TimeMorning {
		t.Fatalf("night pidgey = %+v, want closed until morning", pidgey)
	}
	if pidgey.MinLevel != 2 || pidgey.MaxLevel != 4 || pidgey.Slots != 3 {
		t.Fatalf("night pidgey band = %+v, want levels 2-4 across 3 slots", pidgey)
	}
	sentret := speciesClock(t, night, "sentret")
	if sentret.Open || sentret.Wait == nil || sentret.Wait.Until != TimeMorning {
		t.Fatalf("night sentret = %+v, want closed until morning", sentret)
	}
	if hasSpeciesClock(night, "rattata") || hasSpeciesClock(night, "magikarp") {
		t.Fatalf("ungated species leaked into the night plan: %+v", night)
	}

	morning := PlanSpeciesClocks(TimeMorning, slots)
	open := speciesClock(t, morning, "pidgey")
	if !open.Open || open.Wait != nil {
		t.Fatalf("morning pidgey = %+v, want open with no wait", open)
	}
	sentretMorning := speciesClock(t, morning, "sentret")
	if !sentretMorning.Open || sentretMorning.Wait != nil {
		t.Fatalf("morning sentret = %+v, want open with no wait", sentretMorning)
	}
}

func TestPlanSpeciesClocksWrapsToMorningFromDay(t *testing.T) {
	slots := []TimedSlot{{Species: "ledyba", Level: 5, Times: []TimeOfDay{TimeMorning}}}
	got := PlanSpeciesClocks(TimeDay, slots)
	ledyba := speciesClock(t, got, "ledyba")
	if ledyba.Open || ledyba.Wait == nil || ledyba.Wait.Until != TimeMorning {
		t.Fatalf("day ledyba = %+v, want closed until morning", ledyba)
	}
}

func TestPlanSpeciesClocksDoesNotInventAWaitWithoutAClock(t *testing.T) {
	slots := []TimedSlot{{Species: "pidgey", Level: 2, Times: []TimeOfDay{TimeMorning}}}
	got := PlanSpeciesClocks("", slots)
	pidgey := speciesClock(t, got, "pidgey")
	if pidgey.Open || pidgey.Wait != nil {
		t.Fatalf("clockless pidgey = %+v, want closed and no invented wait", pidgey)
	}
}

func TestPlanSpeciesClocksLeavesUngatedSpeciesOut(t *testing.T) {
	slots := []TimedSlot{{Species: "pidgey", Level: 2}}
	if got := PlanSpeciesClocks(TimeNight, slots); got != nil {
		t.Fatalf("ungated plan = %+v, want none", got)
	}
}

func speciesClock(t *testing.T, got []SpeciesClock, id SpeciesID) SpeciesClock {
	t.Helper()
	for _, sc := range got {
		if sc.Species == id {
			return sc
		}
	}
	t.Fatalf("species %q missing from %+v", id, got)
	return SpeciesClock{}
}

func hasSpeciesClock(got []SpeciesClock, id SpeciesID) bool {
	for _, sc := range got {
		if sc.Species == id {
			return true
		}
	}
	return false
}
