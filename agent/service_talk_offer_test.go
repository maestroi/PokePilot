package agent

import "testing"

func TestFilterRedServiceTalkObjectivesSuppressesMagikarpSalesman(t *testing.T) {
	objectives := []Objective{
		{Kind: KindTalk, X: mtMoonMagikarpSalesmanHomeX, Y: mtMoonMagikarpSalesmanHomeY},
		{Kind: KindTalk, X: 7, Y: 3}, // ordinary gentleman in the same center
		{Kind: KindGoTo, Place: "route 4"},
	}

	// A Pokemon Center is not a Mart, so MartClerkPosition deliberately fails
	// with this nil ROM. The scripted service filter must still run instead of
	// returning the unfiltered objective set.
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
