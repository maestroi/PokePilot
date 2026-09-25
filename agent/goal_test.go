package agent

import "testing"

func TestParseGoal(t *testing.T) {
	cases := []struct {
		in   string
		kind GoalKind
	}{
		{"", GoalNone},
		{"elite-four", GoalEliteFour},
		{"dex", GoalDex},
		{"pokedex", GoalDex},
		{"pokédex", GoalDex},
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

func TestPlannerGoalRecognizesDexPreset(t *testing.T) {
	for _, raw := range []string{
		"dex",
		"Complete the obtainable Pokedex.",
		"Complete the obtainable Pokédex.",
	} {
		g, deterministic, err := PlannerGoal(raw)
		if err != nil {
			t.Fatalf("PlannerGoal(%q): %v", raw, err)
		}
		if !deterministic {
			t.Fatalf("PlannerGoal(%q) resolved as prompt-only", raw)
		}
		if g.Kind != GoalDex {
			t.Fatalf("PlannerGoal(%q).Kind = %v, want GoalDex", raw, g.Kind)
		}
	}
}

func TestEvaluateDexGoalUsesCatalogTargets(t *testing.T) {
	g, err := ParseGoal("dex")
	if err != nil {
		t.Fatalf("ParseGoal: %v", err)
	}

	obs := Observation{Dex: DexCatalog{
		Owned: []DexEntry{
			{Species: "bulbasaur", Owned: true},
			{Species: "ivysaur", Owned: true},
		},
		Targets: []DexEntry{
			{Species: "venusaur"},
			{Species: "pikachu"},
		},
		Unavailable: []DexEntry{
			{Species: "mew", Unavailable: UnavailableEventOnly},
		},
	}}
	status := EvaluateGoal(g, obs)
	if status.Complete {
		t.Fatalf("Dex goal completed with remaining targets: %+v", status)
	}
	if status.Current != 2 || status.Target != 4 {
		t.Fatalf("Dex progress = %d/%d, want 2/4", status.Current, status.Target)
	}

	obs.Dex.Owned = append(obs.Dex.Owned, obs.Dex.Targets...)
	obs.Dex.Targets = nil
	status = EvaluateGoal(g, obs)
	if !status.Complete {
		t.Fatalf("Dex goal did not complete when all obtainable targets were owned: %+v", status)
	}
	if status.Current != 4 || status.Target != 4 {
		t.Fatalf("completed Dex progress = %d/%d, want 4/4", status.Current, status.Target)
	}
}

func TestEvaluateDexGoalNeverCompletesWithoutCatalog(t *testing.T) {
	g, err := ParseGoal("dex")
	if err != nil {
		t.Fatalf("ParseGoal: %v", err)
	}
	if got := EvaluateGoal(g, Observation{}); got.Complete {
		t.Fatalf("empty Dex catalog falsely completed goal: %+v", got)
	}
}

func TestEliteFourRequiresEightBadgesAndHallOfFame(t *testing.T) {
	g, _ := ParseGoal("elite-four")
	eight := []string{"1", "2", "3", "4", "5", "6", "7", "8"}
	obs := Observation{Badges: eight}
	if got := EvaluateGoal(g, obs); got.Complete {
		t.Fatal("eight badges alone completed elite-four goal")
	} else if got.Current != 8 || got.Target != 9 {
		t.Fatalf("eight-badge campaign progress = %d/%d, want 8/9 before Hall of Fame", got.Current, got.Target)
	}

	// Beating the Champion is intentionally not the final goal fact: Red sets
	// that event before Oak escorts the player into the Hall of Fame.
	obs.Story = ProgressState{{ID: ProgressLeagueChampionDefeated, Complete: true}}
	if got := EvaluateGoal(g, obs); got.Complete {
		t.Fatal("champion progress fact completed elite-four goal before Hall of Fame")
	}

	obs.Story = append(obs.Story, ProgressFact{ID: ProgressMainStoryComplete, Complete: true})
	if got := EvaluateGoal(g, obs); !got.Complete {
		t.Fatal("eight badges plus main-story completion did not complete elite-four goal")
	} else if got.Current != 9 || got.Target != 9 {
		t.Fatalf("completed elite-four progress = %d/%d, want 9/9", got.Current, got.Target)
	}

	// #39 is a fresh-campaign qualification, so a synthetic ending flag without
	// the badge journey must not qualify even though the ending itself occurred.
	mainStoryOnly := Observation{Story: ProgressState{{ID: ProgressMainStoryComplete, Complete: true}}}
	if got := EvaluateGoal(g, mainStoryOnly); got.Complete {
		t.Fatalf("main-story bit without eight badges qualified the full campaign: %+v", got)
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

// The elite-four predicate is intentionally pinned to portable semantic facts
// crossing the game-adapter boundary. Falling back to Observation.Events here
// would make Red event spelling part of the generic planner contract again.
func TestEvaluateGoalEliteFourCompletesOnSemanticProgress(t *testing.T) {
	g, err := ParseGoal("elite-four")
	if err != nil {
		t.Fatalf("ParseGoal: %v", err)
	}
	eight := []string{"Boulder", "Cascade", "Thunder", "Rainbow", "Soul", "Marsh", "Volcano", "Earth"}

	done := Observation{
		Badges: eight,
		Story:  ProgressState{{ID: ProgressMainStoryComplete, Complete: true}},
	}
	if got := EvaluateGoal(g, done); !got.Complete {
		t.Fatalf("elite-four not complete with eight badges + main-story progress set: %+v", got)
	}

	championOnly := Observation{
		Badges: eight,
		Story:  ProgressState{{ID: ProgressLeagueChampionDefeated, Complete: true}},
	}
	if got := EvaluateGoal(g, championOnly); got.Complete {
		t.Fatalf("elite-four completed before Hall of Fame: %+v", got)
	}

	legacyOnly := Observation{Badges: eight, Events: []string{"BeatChampionRival"}}
	if got := EvaluateGoal(g, legacyOnly); got.Complete {
		t.Fatalf("elite-four completed from legacy Red event text: %+v", got)
	}

	if got := EvaluateGoal(g, Observation{Badges: []string{"Boulder"}}); got.Complete {
		t.Fatalf("elite-four complete without the main-story progress fact: %+v", got)
	}
}
