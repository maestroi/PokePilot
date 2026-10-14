package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
)

func TestClockPeriodMapsWildTableSlots(t *testing.T) {
	cases := []struct {
		raw  string
		want game.TimeOfDay
		ok   bool
	}{
		{TimeMorning, game.TimeMorning, true},
		{TimeDay, game.TimeDay, true},
		{TimeNight, game.TimeNight, true},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := ClockPeriod(tc.raw)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("ClockPeriod(%q) = %q, %v; want %q, %v", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRoute29NightWaitsUntilMorningForPidgey(t *testing.T) {
	rom := loadGold(t)
	info, ok := gsdata.MapByName("ROUTE_29")
	if !ok {
		t.Fatal("ROUTE_29 missing from the map catalog")
	}
	rows, err := EncountersOnMap(rom, gsdata.NativeMapID(info.Group, info.Number))
	if err != nil {
		t.Fatalf("EncountersOnMap(ROUTE_29): %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("ROUTE_29 has no wild rows")
	}

	night := game.PlanSpeciesClocks(game.TimeNight, ClockSlots(rows))
	pidgey := route29Clock(t, night, "pidgey")
	if pidgey.Open || pidgey.Wait == nil || pidgey.Wait.Until != game.TimeMorning {
		t.Fatalf("night pidgey = %+v, want closed until morning", pidgey)
	}
	sentret := route29Clock(t, night, "sentret")
	if sentret.Open || sentret.Wait == nil || sentret.Wait.Until != game.TimeMorning {
		t.Fatalf("night sentret = %+v, want closed until morning", sentret)
	}
	hoothoot := route29Clock(t, night, "hoothoot")
	if !hoothoot.Open || hoothoot.Wait != nil {
		t.Fatalf("night hoothoot = %+v, want open", hoothoot)
	}
	if route29Has(night, "rattata") {
		t.Fatalf("all-day rattata was treated as clock-gated: %+v", night)
	}

	morning := game.PlanSpeciesClocks(game.TimeMorning, ClockSlots(rows))
	open := route29Clock(t, morning, "pidgey")
	if !open.Open || open.Wait != nil {
		t.Fatalf("morning pidgey = %+v, want open with no wait", open)
	}
	closed := route29Clock(t, morning, "hoothoot")
	if closed.Open || closed.Wait == nil || closed.Wait.Until != game.TimeNight {
		t.Fatalf("morning hoothoot = %+v, want closed until night", closed)
	}
}

func route29Clock(t *testing.T, got []game.SpeciesClock, id game.SpeciesID) game.SpeciesClock {
	t.Helper()
	for _, sc := range got {
		if sc.Species == id {
			return sc
		}
	}
	t.Fatalf("species %q missing from %+v", id, got)
	return game.SpeciesClock{}
}

func route29Has(got []game.SpeciesClock, id game.SpeciesID) bool {
	for _, sc := range got {
		if sc.Species == id {
			return true
		}
	}
	return false
}
