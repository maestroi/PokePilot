package agent

import (
	"os"
	"testing"
)

func TestFilterRedServiceTalkObjectivesSuppressesMagikarpSalesman(t *testing.T) {
	objectives := []Objective{
		{Kind: KindTalk, X: mtMoonMagikarpSalesmanHomeX, Y: mtMoonMagikarpSalesmanHomeY},
		{Kind: KindTalk, X: 7, Y: 3}, // ordinary gentleman in the same center
		{Kind: KindGoTo, Place: "route 4"},
	}

	// A Pokemon Center is not a Mart, so ROM service discovery deliberately
	// fails with this nil ROM. The explicit scripted-choice filter must still
	// run instead of returning the unfiltered objective set.
	got := filterRedServiceTalkObjectives(nil, Observation{Map: mtMoonPokecenterMapID}, objectives)
	if len(got) != 2 {
		t.Fatalf("filtered objective count = %d, want 2: %+v", len(got), got)
	}
	for _, o := range got {
		if o.Kind == KindTalk && o.X == mtMoonMagikarpSalesmanHomeX && o.Y == mtMoonMagikarpSalesmanHomeY {
			t.Fatalf("Magikarp salesman was still offered as a generic talk objective: %+v", got)
		}
	}
	if got[0] != (Objective{Kind: KindTalk, X: 7, Y: 3}) {
		t.Fatalf("ordinary Pokemon Center NPC was filtered too: got[0]=%+v", got[0])
	}
}

func TestFilterRedServiceTalkObjectivesSuppressesRomServices(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	objectives := []Objective{
		{Kind: KindTalk, X: 3, Y: 1},  // Viridian Center nurse
		{Kind: KindTalk, X: 10, Y: 5}, // ordinary gentleman
		{Kind: KindTalk, X: 4, Y: 3},  // ordinary cooltrainer
		{Kind: KindTalk, X: 11, Y: 2}, // cable-club receptionist
		{Kind: KindGoTo, Place: "viridian city"},
	}
	got := filterRedServiceTalkObjectives(romData, Observation{Map: 0x29}, objectives)
	want := []Objective{
		{Kind: KindTalk, X: 10, Y: 5},
		{Kind: KindTalk, X: 4, Y: 3},
		{Kind: KindGoTo, Place: "viridian city"},
	}
	if len(got) != len(want) {
		t.Fatalf("filtered objectives = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("filtered objectives[%d] = %+v, want %+v (all=%+v)", i, got[i], want[i], got)
		}
	}
}

func TestRedOwnedChoiceActorDoesNotMatchOrdinaryNPC(t *testing.T) {
	if !redOwnedChoiceActor(mtMoonPokecenterMapID, mtMoonMagikarpSalesmanHomeX, mtMoonMagikarpSalesmanHomeY) {
		t.Fatal("Magikarp salesman should be classified as an owned choice actor")
	}
	if redOwnedChoiceActor(mtMoonPokecenterMapID, 7, 3) {
		t.Fatal("ordinary Mt. Moon Pokemon Center gentleman was classified as an owned choice actor")
	}
	if redOwnedChoiceActor(0x43, mtMoonMagikarpSalesmanHomeX, mtMoonMagikarpSalesmanHomeY) {
		t.Fatal("same coordinates on another map were classified as the Magikarp salesman")
	}
}
