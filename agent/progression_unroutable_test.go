package agent

import "testing"

type fixedProgressionPlanner []Objective

func (p fixedProgressionPlanner) ProgressionObjectives(Observation) []Objective {
	return []Objective(p)
}

func TestOfferWithProgressionDropsKnownBadJourneysWhenStoryCanAdvance(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{
		0x3B: {0x0F, 0x3C},
		0x0F: {0x3B},
		0x3C: {0x3B},
	})
	known.SawMap(0x3B)
	known.SawMap(0x0F)
	known.SawMap(0x3C)

	obs := Observation{
		Map:        0x3B,
		MapName:    "MT_MOON_1F",
		X:          5,
		Y:          5,
		PartyCount: 1,
		Unroutable: []string{"mt moon 1f", "mt moon b1f", "route 4"},
	}

	// Generic Offer keeps its fail-open safety valve when every live-route
	// answer is negative.
	generic := Offer(obs, known)
	genericJourneys := 0
	for _, o := range generic {
		if o.Kind == KindGoTo {
			genericJourneys++
		}
	}
	if genericJourneys == 0 {
		t.Fatal("test assumption wrong: generic Offer no longer fail-opens all-unroutable travel")
	}

	planner := fixedProgressionPlanner{{Kind: KindProgress, Progress: redProgressMtMoonFossilAcquired}}
	offered := OfferWithProgression(obs, known, planner)
	progressFound := false
	for _, o := range offered {
		if o.Kind == KindGoTo {
			t.Errorf("known-unroutable journey survived next to a valid progression objective: %v", o)
		}
		if o.Kind == KindProgress && o.Progress == redProgressMtMoonFossilAcquired {
			progressFound = true
		}
	}
	if !progressFound {
		t.Fatalf("progression objective disappeared while filtering journeys: %v", offered)
	}
}
