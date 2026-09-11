package agent

import (
	"strings"
	"testing"
)

// TestOfferLastResortNoteOnlyWhenNothingElseIsOffered: a stuck-position replan
// (run-g9ojxmtgvrff1ezck9g7t1o7x) showed the strategist repeatedly retrying
// already-failed local objectives with no signal that fighting to a loss —
// not fleeing, not retreating — is itself a legal way out. The note must
// appear only when Train is genuinely the only thing on offer, never when a
// journey, gym, or trainer challenge is still legally reachable.
func TestOfferLastResortNoteOnlyWhenNothingElseIsOffered(t *testing.T) {
	known := NewKnowledge(nil)

	stuck := Observation{
		Map: 0x17, MapName: "ROUTE_12", PartyCount: 1,
		Party:        []PartyMon{{Level: 31, HP: 86, MaxHP: 86}},
		HasGrass:     true,
		WildGrass:    []WildSpecies{{Name: "RATTATA", MinLevel: 3, MaxLevel: 5}},
		RespawnPlace: "vermilion city",
	}
	objs := Offer(stuck, known)
	var train *Objective
	for i := range objs {
		if objs[i].Kind == KindTrain {
			train = &objs[i]
		}
		if objs[i].Kind == KindGoTo || objs[i].Kind == KindGym || objs[i].Kind == KindTrainer {
			t.Fatalf("test fixture is not actually stuck: got %s", objs[i])
		}
	}
	if train == nil {
		t.Fatal("no KindTrain objective offered on a stuck map with reachable grass")
	}
	if !strings.Contains(strings.ToLower(train.Note), "vermilion city") {
		t.Errorf("Train.Note = %q, want it to mention the respawn place as the last-resort escape", train.Note)
	}

	notStuck := stuck
	notStuck.Map = 0x02 // Pewter City: Brock's gym is always offered pre-Boulder-Badge
	notStuck.MapName = "PEWTER_CITY"
	for _, o := range Offer(notStuck, known) {
		if o.Kind == KindTrain && strings.Contains(o.Note, "vermilion city") {
			t.Errorf("Train.Note = %q, want no last-resort note while the gym is still offered", o.Note)
		}
	}
}
