package agent

import (
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestProjectClockEncountersWaitsUntilMorningAtNight(t *testing.T) {
	slots := []game.TimedSlot{
		{Species: "pidgey", Level: 2, Times: []game.TimeOfDay{game.TimeMorning}},
		{Species: "pidgey", Level: 4, Times: []game.TimeOfDay{game.TimeDay}},
		{Species: "hoothoot", Level: 3, Times: []game.TimeOfDay{game.TimeNight}},
		{Species: "rattata", Level: 2, Times: []game.TimeOfDay{game.TimeMorning}},
		{Species: "rattata", Level: 2, Times: []game.TimeOfDay{game.TimeDay}},
		{Species: "rattata", Level: 2, Times: []game.TimeOfDay{game.TimeNight}},
	}

	night := projectClockEncounters(game.TimeNight, slots)
	pidgey := clockEncounter(t, night, "pidgey")
	if pidgey.Open || pidgey.Until != string(game.TimeMorning) {
		t.Fatalf("night pidgey = %+v, want closed until morning", pidgey)
	}
	hoothoot := clockEncounter(t, night, "hoothoot")
	if !hoothoot.Open || hoothoot.Until != "" {
		t.Fatalf("night hoothoot = %+v, want open", hoothoot)
	}
	if hasClockEncounter(night, "rattata") {
		t.Fatalf("all-day rattata leaked into clock encounters: %+v", night)
	}

	morning := projectClockEncounters(game.TimeMorning, slots)
	open := clockEncounter(t, morning, "pidgey")
	if !open.Open || open.Until != "" {
		t.Fatalf("morning pidgey = %+v, want open with no wait", open)
	}
	closed := clockEncounter(t, morning, "hoothoot")
	if closed.Open || closed.Until != string(game.TimeNight) {
		t.Fatalf("morning hoothoot = %+v, want closed until night", closed)
	}
}

func TestObservationJSONStatesTheWaitUntilMorningLeg(t *testing.T) {
	obs := Observation{
		TimeOfDay: string(game.TimeNight),
		ClockEncounters: []ClockEncounter{{
			Species:  "pidgey",
			Open:     false,
			Until:    string(game.TimeMorning),
			MinLevel: 2,
			MaxLevel: 4,
		}},
	}
	raw, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var got struct {
		ClockEncounters []ClockEncounter `json:"clock_encounters"`
		DecisionContext *DecisionContext
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(got.ClockEncounters) != 1 || got.ClockEncounters[0].Open || got.ClockEncounters[0].Until != "morning" {
		t.Fatalf("clock_encounters = %+v, want closed pidgey until morning", got.ClockEncounters)
	}
	if got.DecisionContext == nil || got.DecisionContext.Clock == nil || len(got.DecisionContext.Clock.Waits) != 1 {
		t.Fatalf("DecisionContext.Clock = %+v, want one wait", got.DecisionContext)
	}
	wait := got.DecisionContext.Clock.Waits[0]
	if wait.Species != "pidgey" || wait.Until != "morning" {
		t.Fatalf("wait = %+v, want pidgey until morning", wait)
	}
}

func clockEncounter(t *testing.T, got []ClockEncounter, species string) ClockEncounter {
	t.Helper()
	for _, enc := range got {
		if enc.Species == species {
			return enc
		}
	}
	t.Fatalf("species %q missing from %+v", species, got)
	return ClockEncounter{}
}

func hasClockEncounter(got []ClockEncounter, species string) bool {
	for _, enc := range got {
		if enc.Species == species {
			return true
		}
	}
	return false
}
