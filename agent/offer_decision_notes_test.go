package agent

import (
	"strings"
	"testing"
)

func TestOfferMarksUnvisitedAdjacentJourney(t *testing.T) {
	adj := map[uint8][]uint8{
		0x02: {0x0d, 0x0e, 0x36, 0x3a},
	}
	known := NewKnowledge(adj)
	for _, id := range []uint8{0x02, 0x0d, 0x36, 0x3a} {
		known.SawMap(id)
	}
	obs := Observation{
		Map:        0x02,
		MapName:    "PEWTER_CITY",
		X:          14,
		Y:          8,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 20, HP: 50, MaxHP: 50}},
	}

	offered := Offer(obs, known)
	for _, objective := range []string{
		"go to route 3",
		"go to route 3, fleeing wild battles",
	} {
		note, ok := offeredNote(offered, objective)
		if !ok {
			t.Fatalf("%q was not offered", objective)
		}
		if !strings.Contains(note, "unvisited adjacent map") {
			t.Fatalf("%q note = %q, want unvisited-adjacent fact", objective, note)
		}
	}

	if note, ok := offeredNote(offered, "go to route 2"); !ok {
		t.Fatal("visited route 2 was not offered")
	} else if strings.Contains(note, "unvisited adjacent map") {
		t.Fatalf("visited route 2 note = %q", note)
	}
}

func TestOfferPutsLocalWildBandOnTrainChoiceAndKeepsHistory(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{0x0d: {0x02}})
	known.SawMap(0x0d)
	train := Objective{Kind: KindTrain, Level: 22}.String()
	known.Failures[train] = Failure{Objective: train, Times: 1, Last: "target not reached"}

	obs := Observation{
		Map:        0x0d,
		MapName:    "ROUTE_2",
		PartyCount: 1,
		Party:      []PartyMon{{Level: 20, HP: 50, MaxHP: 50}},
		HasGrass:   true,
		WildGrass: []WildSpecies{
			{Name: "pidgey", MinLevel: 3, MaxLevel: 5, Slots: 6},
			{Name: "rattata", MinLevel: 4, MaxLevel: 7, Slots: 4},
		},
	}

	note, ok := offeredNote(Offer(obs, known), train)
	if !ok {
		t.Fatalf("%q was not offered", train)
	}
	for _, want := range []string{"lead L20", "local wilds L3-L7", "failed 1x"} {
		if !strings.Contains(note, want) {
			t.Fatalf("train note = %q, want %q", note, want)
		}
	}
}

func offeredNote(offered []Objective, want string) (string, bool) {
	for _, o := range offered {
		if o.String() == want {
			return o.Note, true
		}
	}
	return "", false
}
