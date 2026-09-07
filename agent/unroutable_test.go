package agent

import "testing"

// TestOfferWithholdsUnroutableJourneys: the router said no, so the menu must
// not offer it. MEASURED 2026-09-07 on run-3t5kvlk55zvbjkkkno3ses4g6, where
// "go to viridian city" was the most-picked objective of the run and every
// pick died on `world: no route` from Mt. Moon 1F's (5,5) ladder landing.
func TestOfferWithholdsUnroutableJourneys(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{
		0x3B: {0x0F},
		0x0F: {0x3B, 0x03},
	})
	known.SawMap(0x3B)
	known.SawMap(0x0F)
	known.SawMap(0x01)

	obs := Observation{Map: 0x3B, MapName: "MT_MOON_1F", X: 5, Y: 5, PartyCount: 1}
	if !offersPlace(Offer(obs, known), "viridian city") {
		t.Fatal("test assumption wrong: Viridian is not on the menu even before the router is consulted")
	}

	obs.Unroutable = []string{"viridian city"}
	if offersPlace(Offer(obs, known), "viridian city") {
		t.Error("Viridian stayed on the menu after the router said there is no route to it")
	}
	if !offersPlace(Offer(obs, known), "mt moon 1f") {
		t.Error("withholding one unroutable place took a routable one with it")
	}
}

// TestOfferFailsOpenWhenRoutabilityWasNeverAsked: nil Unroutable means the
// question was never put to the router (no ROM, a graph that failed to
// build). That is not evidence, and it must never empty the menu.
func TestOfferFailsOpenWhenRoutabilityWasNeverAsked(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{0x3B: {0x0F}, 0x0F: {0x3B}})
	known.SawMap(0x3B)
	known.SawMap(0x0F)

	obs := Observation{Map: 0x3B, MapName: "MT_MOON_1F", X: 5, Y: 5, PartyCount: 1}
	obs.Unroutable = nil
	if !offersPlace(Offer(obs, known), "route 4") {
		t.Error("a nil Unroutable withheld a place; it must fail open")
	}
}

// TestLogUnroutableRecordsOncePerChange: the withheld place has to reach a
// human, and standing still must not spam the log.
func TestLogUnroutableRecordsOncePerChange(t *testing.T) {
	var buf writerFunc
	prev := ""
	obs := Observation{Map: 0x3B, X: 5, Y: 5, Unroutable: []string{"cerulean city", "viridian city"}}

	logUnroutable(&buf, 7, obs, &prev)
	logUnroutable(&buf, 8, obs, &prev) // unchanged: silent
	if got := len(buf.lines); got != 1 {
		t.Fatalf("logged %d lines for an unchanged set, want 1: %v", got, buf.lines)
	}
	want := "round 7: unroutable from map 3b at (5,5): cerulean city, viridian city\n"
	if buf.lines[0] != want {
		t.Errorf("line = %q, want %q", buf.lines[0], want)
	}

	obs.Unroutable = []string{"cerulean city"}
	logUnroutable(&buf, 9, obs, &prev)
	if len(buf.lines) != 2 {
		t.Errorf("a changed set did not log: %v", buf.lines)
	}

	// Nothing unroutable is not news.
	obs.Unroutable = []string{}
	logUnroutable(&buf, 10, obs, &prev)
	if len(buf.lines) != 2 {
		t.Errorf("an empty set logged a line: %v", buf.lines)
	}
}

type writerFunc struct{ lines []string }

func (w *writerFunc) Write(p []byte) (int, error) {
	w.lines = append(w.lines, string(p))
	return len(p), nil
}

// TestOfferNeverEmptiesTheMenuOnUnroutability: the routability answer comes
// from the live map overlay, and a wrong overlay is wrong about every
// destination on that map at once. Filtering on it must not be able to leave
// the planner with nothing to pick.
func TestOfferNeverEmptiesTheMenuOnUnroutability(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{0x3B: {0x0F, 0x3C}, 0x0F: {0x3B}, 0x3C: {0x3B}})
	known.SawMap(0x3B)
	known.SawMap(0x0F)
	known.SawMap(0x3C)

	obs := Observation{Map: 0x3B, MapName: "MT_MOON_1F", X: 5, Y: 5, PartyCount: 1}
	all := Offer(obs, known)

	// The router claims nothing at all is reachable.
	obs.Unroutable = []string{"mt moon 1f", "mt moon b1f", "route 4"}
	kept := Offer(obs, known)

	journeys := 0
	for _, o := range kept {
		if o.Kind == KindGoTo {
			journeys++
		}
	}
	if journeys == 0 {
		t.Fatalf("every journey was withheld, leaving the planner nothing; had %d before", len(all))
	}
}
