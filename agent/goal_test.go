package agent

import "testing"

func TestParseGoal(t *testing.T) {
	cases := []struct {
		in   string
		kind GoalKind
	}{
		{"", GoalNone},
		{"elite-four", GoalEliteFour},
		{"badges:8", GoalBadges},
		{"reach:Cerulean City", GoalReach},
		{"level:25", GoalLevel},
		{"item:Potion", GoalItem},
	}
	for _, tc := range cases {
		g, err := ParseGoal(tc.in)
		if err != nil {
			t.Fatalf("ParseGoal(%q): %v", tc.in, err)
		}
		if g.Kind != tc.kind {
			t.Fatalf("ParseGoal(%q).Kind = %v, want %v", tc.in, g.Kind, tc.kind)
		}
	}
}

func TestParseGoalRejectsInvalidTargets(t *testing.T) {
	for _, in := range []string{"unknown:x", "badges:0", "badges:9", "level:101", "reach:"} {
		if _, err := ParseGoal(in); err == nil {
			t.Fatalf("ParseGoal(%q) unexpectedly succeeded", in)
		}
	}
}

func TestEliteFourRequiresHallOfFameProgressFact(t *testing.T) {
	g, _ := ParseGoal("elite-four")
	obs := Observation{Badges: []string{"1", "2", "3", "4", "5", "6", "7", "8"}}
	if got := EvaluateGoal(g, obs); got.Complete {
		t.Fatal("eight badges alone completed elite-four goal")
	}

	// Beating the Champion is intentionally not the final goal fact: Red sets
	// that event before Oak escorts the player into the Hall of Fame.
	obs.Story = ProgressState{{ID: ProgressLeagueChampionDefeated, Complete: true}}
	if got := EvaluateGoal(g, obs); got.Complete {
		t.Fatal("champion progress fact completed elite-four goal before Hall of Fame")
	}

	obs.Story = append(obs.Story, ProgressFact{ID: ProgressMainStoryComplete, Complete: true})
	if got := EvaluateGoal(g, obs); !got.Complete {
		t.Fatal("main-story completion fact did not complete elite-four goal")
	}
}

func TestEvaluateGoalUsesObservableState(t *testing.T) {
	g, _ := ParseGoal("level:25")
	obs := Observation{Party: []PartyMon{{Level: 18}, {Level: 25}}}
	if got := EvaluateGoal(g, obs); !got.Complete || got.Current != 25 {
		t.Fatalf("level status = %+v", got)
	}

	g, _ = ParseGoal("item:potion")
	obs = Observation{Bag: []Item{{Name: "POTION", Quantity: 2}}}
	if got := EvaluateGoal(g, obs); !got.Complete {
		t.Fatalf("item status = %+v", got)
	}

	g, _ = ParseGoal("reach:cerulean city")
	obs = Observation{MapName: "Cerulean City"}
	if got := EvaluateGoal(g, obs); !got.Complete {
		t.Fatalf("reach status = %+v", got)
	}
}

// The elite-four predicate is intentionally pinned to the semantic fact that
// crosses the game-adapter boundary. Falling back to Observation.Events here
// would make a Red event spelling part of the generic planner contract again.
func TestEvaluateGoalEliteFourCompletesOnSemanticProgress(t *testing.T) {
	g, err := ParseGoal("elite-four")
	if err != nil {
		t.Fatalf("ParseGoal: %v", err)
	}

	done := Observation{Story: ProgressState{{ID: ProgressMainStoryComplete, Complete: true}}}
	if got := EvaluateGoal(g, done); !got.Complete {
		t.Fatalf("elite-four not complete with main-story progress set: %+v", got)
	}

	championOnly := Observation{Story: ProgressState{{ID: ProgressLeagueChampionDefeated, Complete: true}}}
	if got := EvaluateGoal(g, championOnly); got.Complete {
		t.Fatalf("elite-four completed before Hall of Fame: %+v", got)
	}

	legacyOnly := Observation{Events: []string{"BeatChampionRival"}}
	if got := EvaluateGoal(g, legacyOnly); got.Complete {
		t.Fatalf("elite-four completed from legacy Red event text: %+v", got)
	}

	if got := EvaluateGoal(g, Observation{Badges: []string{"Boulder"}}); got.Complete {
		t.Fatalf("elite-four complete without the main-story progress fact: %+v", got)
	}
}
